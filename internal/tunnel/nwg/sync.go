// Runtime field-level sync methods for OperatorNativeWG.
//
// These are called from service.applyDiffNWG to push specific stored
// fields (DNS, address/MTU, peer, AWG params, description) to a running
// NDMS interface without restarting it. They are decoupled from the
// lifecycle (Create/Start/Stop/Delete), which lives in operator.go and
// owns the heavier orchestration around kmod, peer-via, etc.
package nwg

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/ndms/payloads"
	"github.com/hoaxisr/awg-manager/internal/storage"
	"github.com/hoaxisr/awg-manager/internal/sys/ndmsinfo"
)

// SyncDNS reconciles DNS servers for a NativeWG tunnel: clears oldDNS
// from NDMS, then applies newDNS. Either side may be nil/empty —
// passing both lists explicitly avoids needing applied-state tracking.
//
// Contract asymmetry vs OperatorOS5Impl.SyncDNS(ctx, id, dns): the OS5
// path tracks applied DNS internally and computes its own diff. The NWG
// path takes both lists as parameters and is stateless. This is
// deliberate — caller already knows oldDNS (it's the previous stored
// value), so the diff naturally lives at the call site.
//
// Use cases:
//   - Start tunnel: SyncDNS(ctx, stored, nil, tunnel.ParseDNSList(stored.Interface.DNS))
//   - Stop tunnel:  SyncDNS(ctx, stored, tunnel.ParseDNSList(stored.Interface.DNS), nil)
//   - Update DNS:   SyncDNS(ctx, stored, oldList, newList)
func (o *OperatorNativeWG) SyncDNS(ctx context.Context, stored *storage.AWGTunnel, oldDNS, newDNS []string) error {
	names := NewNWGNames(stored.NWGIndex)
	if len(oldDNS) > 0 {
		if err := o.commands.Interfaces.ClearDNS(ctx, names.NDMSName, oldDNS); err != nil {
			o.appLog.Warn("clear-dns", names.NDMSName, err.Error())
		}
	}
	if len(newDNS) > 0 {
		if err := o.commands.Interfaces.SetDNS(ctx, names.NDMSName, newDNS); err != nil {
			return fmt.Errorf("set DNS: %w", err)
		}
	}
	return nil
}

// SyncAWGParams applies AmneziaWG obfuscation parameters (Jc, Jmin,
// Jmax, S1-S4, H1-H4, I1-I5, Qlen) to a running NativeWG tunnel via
// RCI. Best-effort: if NDMS rejects (some firmware versions require
// interface down for ASC changes), failures bubble up so the caller
// can log a Warn and instruct the user to restart the tunnel.
func (o *OperatorNativeWG) SyncAWGParams(ctx context.Context, stored *storage.AWGTunnel) error {
	if !ndmsinfo.SupportsWireguardASC() {
		return fmt.Errorf("ASC not supported by firmware; restart tunnel to apply")
	}
	names := NewNWGNames(stored.NWGIndex)
	ascJSON, err := buildASCJSON(&stored.Interface)
	if err != nil {
		return fmt.Errorf("build ASC params: %w", err)
	}
	if ascJSON == nil {
		return nil
	}
	if err := o.commands.Wireguard.SetASCParams(ctx, names.NDMSName, ascJSON); err != nil {
		return fmt.Errorf("set ASC params: %w", err)
	}
	return nil
}

// SyncAddressMTU pushes the stored address and MTU to the NDMS interface.
// Called on Start (to override any changes made via the router UI)
// and on Update (to hot-apply changes to a running tunnel).
func (o *OperatorNativeWG) SyncAddressMTU(ctx context.Context, stored *storage.AWGTunnel) error {
	ndmsName := NewNWGNames(stored.NWGIndex).NDMSName
	ipv4 := extractIPv4(stored.Interface.Address)

	addr, mask := splitAddressMask(ipv4)
	if err := o.commands.Interfaces.SetAddress(ctx, ndmsName, addr, mask); err != nil {
		return fmt.Errorf("sync address: %w", err)
	}

	ipv6 := extractIPv6(stored.Interface.Address)
	if ipv6 != "" {
		if err := o.commands.Interfaces.SetIPv6Address(ctx, ndmsName, ipv6); err != nil {
			o.appLog.Warn("sync-address-mtu", ndmsName, "ipv6: "+err.Error())
		}
	} else {
		_ = o.commands.Interfaces.ClearIPv6Address(ctx, ndmsName)
	}

	if err := o.commands.Interfaces.SetMTU(ctx, ndmsName, stored.Interface.MTU); err != nil {
		return fmt.Errorf("sync mtu: %w", err)
	}

	o.appLog.Info("sync-address-mtu", ndmsName, fmt.Sprintf("address=%s ipv6=%s mtu=%d", ipv4, ipv6, stored.Interface.MTU))
	return nil
}

// SyncPrivateKey pushes stored.Interface.PrivateKey to NDMS.
//
// Required when the interface section is replaced wholesale (ReplaceConfig)
// or its PrivateKey changes via Update. CmdWireguardPrivateKey is otherwise
// only emitted in Create — without explicit re-sync, NDMS keeps the original
// key from import. WG kernel then signs handshake initiators with the
// old identity; the new server (whose peer entry expects the public key
// derived from the NEW private key) silently drops them → handshake never
// completes. Symptom: tx grows, rx stays at 0, last-handshake never updates.
func (o *OperatorNativeWG) SyncPrivateKey(ctx context.Context, stored *storage.AWGTunnel) error {
	ndmsName := NewNWGNames(stored.NWGIndex).NDMSName
	cmds := []any{
		payloads.CmdWireguardPrivateKey(ndmsName, stored.Interface.PrivateKey),
		payloads.CmdSave(),
	}
	if _, err := o.transport.PostBatch(ctx, cmds); err != nil {
		return fmt.Errorf("sync private-key: %w", err)
	}
	o.appLog.Info("sync-private-key", ndmsName, "private-key synced")
	return nil
}

// SyncPeer pushes the stored peer configuration to the NDMS interface.
// This applies key/allowed-ips/keepalive/preshared-key from storage.
//
// previousPublicKey lets callers atomically replace the peer when the
// public key changes (e.g. ReplaceConfig from a fresh .conf). If non-
// empty AND different from stored.Peer.PublicKey, the old peer entry is
// removed from NDMS in the same batch as the new one is added — without
// this, NDMS keeps both peers (it indexes by key) and the interface
// ends up with an orphan from the previous config. Pass "" when there
// is no previous peer to remove (e.g. fresh tunnel start).
func (o *OperatorNativeWG) SyncPeer(ctx context.Context, stored *storage.AWGTunnel, previousPublicKey string) error {
	ndmsName := NewNWGNames(stored.NWGIndex).NDMSName
	o.appLog.Full("replace-config", stored.Name, "Syncing peer parameters to NDMS")

	// NDMS отвергает IPv6-endpoint в peer-командах: в RCI уходит заглушка,
	// реальный endpoint живёт в ядре (wg set ниже + endpoint-страж).
	// Hostname резолвим здесь же (v4 предпочтителен — netutil.preferIPv4):
	// AAAA-only-имя NDMS резолвить не умеет, ему тоже нужна заглушка.
	rciEndpoint := stored.Peer.Endpoint
	kernelEndpoint, kernelV6 := canonicalV6Endpoint(stored.Peer.Endpoint)
	v4Confirmed := false
	if !kernelV6 {
		if host, ok := splitEndpointHost(stored.Peer.Endpoint); ok && net.ParseIP(host) != nil {
			v4Confirmed = true // v4-литерал — endpoint'ом управляет сам NDMS
		} else if ip, port, err := o.resolveEndpointWithFallback(stored); err == nil {
			if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() == nil {
				kernelEndpoint = net.JoinHostPort(ip, strconv.Itoa(port))
				kernelV6 = true
			} else {
				v4Confirmed = true
			}
		}
		// Ошибка резолва: ни v4, ни v6 не подтверждены — hostname уходит
		// в RCI как раньше, судьба стража решается ниже.
	}
	if kernelV6 {
		rciEndpoint = ndmsEndpointPlaceholder
	}
	peerCfg := payloads.PeerConfig{
		PublicKey: stored.Peer.PublicKey,
		Endpoint:  rciEndpoint,
	}
	if stored.Peer.PersistentKeepalive > 0 {
		peerCfg.KeepaliveInterval = stored.Peer.PersistentKeepalive
	}
	if stored.Peer.PresharedKey != "" {
		peerCfg.PresharedKey = stored.Peer.PresharedKey
	}

	for _, raw := range stored.Peer.AllowedIPs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if _, netw, err := net.ParseCIDR(s); err == nil && netw != nil {
			ones, _ := netw.Mask.Size()
			item := payloads.AllowedIP{Address: netw.IP.String(), Mask: strconv.Itoa(ones)}
			if netw.IP.To4() != nil {
				peerCfg.AllowedIPv4 = append(peerCfg.AllowedIPv4, item)
			} else {
				peerCfg.AllowedIPv6 = append(peerCfg.AllowedIPv6, item)
			}
			continue
		}
		if ip := net.ParseIP(s); ip != nil {
			if v4 := ip.To4(); v4 != nil {
				peerCfg.AllowedIPv4 = append(peerCfg.AllowedIPv4, payloads.AllowedIP{
					Address: v4.String(),
					Mask:    "32",
				})
			} else {
				peerCfg.AllowedIPv6 = append(peerCfg.AllowedIPv6, payloads.AllowedIP{
					Address: ip.String(),
					Mask:    "128",
				})
			}
		}
	}

	cmds := make([]any, 0, 3)
	if previousPublicKey != "" && previousPublicKey != stored.Peer.PublicKey {
		cmds = append(cmds, payloads.CmdWireguardPeerNo(ndmsName, previousPublicKey))
	}
	cmds = append(cmds, payloads.CmdWireguardPeer(ndmsName, peerCfg), payloads.CmdSave())
	_, err := o.transport.PostBatch(ctx, cmds)
	if err != nil {
		return fmt.Errorf("sync peer: %w", err)
	}

	if stored.ISPInterface != "" {
		if _, err := o.transport.Post(ctx, payloads.CmdWireguardPeerConnect(ndmsName, stored.Peer.PublicKey, stored.ISPInterface)); err != nil {
			o.appLog.Warn("sync-peer", ndmsName, "peer connect via: "+err.Error())
		}
	}

	// Работающий v6-туннель: смена ключа/endpoint'а должна доехать до ядра
	// сразу, а реестр стража — обновиться (иначе он восстановит старые
	// значения поверх новых, а wg set по старому ключу вообще ВОСКРЕШАЕТ
	// удалённого пира на интерфейсе).
	switch {
	case kernelV6 && o.guardHas(stored.ID):
		ifaceName := NewNWGNames(stored.NWGIndex).IfaceName
		if err := setKernelPeerEndpoint(ctx, ifaceName, stored.Peer.PublicKey, kernelEndpoint); err != nil {
			o.appLog.Warn("sync-peer", ndmsName, "kernel endpoint: "+err.Error())
		} else {
			o.guardRegister(stored.ID, guardEntry{
				iface:    ifaceName,
				pubkey:   stored.Peer.PublicKey,
				endpoint: kernelEndpoint,
				spec:     stored.Peer.Endpoint,
				name:     ndmsName,
			})
		}
	case v4Confirmed:
		// Туннель вернулся на v4 — endpoint'ом снова управляет NDMS
		// (реальный адрес ушёл в RCI выше), стражу здесь делать нечего.
		if o.guardHas(stored.ID) {
			o.guardUnregister(stored.ID)
			o.appLog.Info("sync-peer", ndmsName, "endpoint теперь v4 — endpoint-страж снят, адресом управляет NDMS")
		}
	default:
		// Резолв hostname'а не удался. Если ключ или endpoint изменились,
		// реестр стража устарел — снять, иначе он воскресит старого пира.
		if e, ok := o.guardGet(stored.ID); ok &&
			(e.pubkey != stored.Peer.PublicKey || e.spec != stored.Peer.Endpoint) {
			o.guardUnregister(stored.ID)
			o.appLog.Warn("sync-peer", ndmsName,
				"endpoint не резолвится, параметры пира изменились — endpoint-страж снят; если имя резолвится только в IPv6, перезапустите туннель")
		}
	}

	o.appLog.Full("replace-config", stored.Name, "Peer sync complete")
	o.appLog.Info("sync-peer", ndmsName, fmt.Sprintf("allowed v4=%d, v6=%d", len(peerCfg.AllowedIPv4), len(peerCfg.AllowedIPv6)))
	return nil
}

// UpdateDescription updates the NDMS interface description.
func (o *OperatorNativeWG) UpdateDescription(ctx context.Context, stored *storage.AWGTunnel, name string) error {
	return o.commands.Interfaces.SetDescription(ctx, NewNWGNames(stored.NWGIndex).NDMSName, name)
}
