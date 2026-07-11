package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/hoaxisr/awg-manager/internal/logging"
	ndmsquery "github.com/hoaxisr/awg-manager/internal/ndms/query"
	"github.com/hoaxisr/awg-manager/internal/storage"
	"github.com/hoaxisr/awg-manager/internal/tunnel/wan"
)

// logStartup logs system startup information.
// describeListenInterfaces renders the configured bind scope for logs.
func describeListenInterfaces(ifaces []string) string {
	if len(ifaces) == 0 {
		return "all (0.0.0.0)"
	}
	return strings.Join(ifaces, ", ")
}

func logStartup(appLog *logging.ScopedLogger, version, osVersion, listenAddr string, settings *storage.Settings) {
	appLog.Info("startup", "", fmt.Sprintf("AWG Manager v%s started", version))
	appLog.Info("startup", "", fmt.Sprintf("Keenetic OS: %s", osVersion))
	appLog.Info("startup", "", fmt.Sprintf("Listening on %s", listenAddr))

	// Log feature status
	if settings.PingCheck.Enabled {
		appLog.Info("startup", "", "Ping Check: enabled")
	}
	if settings.Logging.Enabled {
		appLog.Info("startup", "", "Logging: enabled")
	}
}

// populateWANModel queries NDMS for current WAN interfaces and fills the
// unified WAN model so that AnyUp() works before any WAN hooks fire.
func populateWANModel(ctx context.Context, queries *ndmsquery.Queries, model *wan.Model, appLog *logging.ScopedLogger) {
	interfaces, err := queries.Interfaces.ListWAN(ctx)
	if err != nil {
		appLog.Warn("populate-wan", "", "failed to get WAN interfaces: "+err.Error())
		return
	}
	model.Populate(interfaces)
	ifaceList := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		ifaceList = append(ifaceList, fmt.Sprintf("%s(up=%t)", iface.Name, iface.Up))
	}
	appLog.Info("populate-wan", "", fmt.Sprintf("WAN model populated, count=%d ifaces=[%s]", len(interfaces), strings.Join(ifaceList, " ")))
}
