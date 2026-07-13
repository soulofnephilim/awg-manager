package router

import (
	"fmt"
	"net/netip"
	"strings"
)

// dnsUpstreamTypes — типы DNS-серверов с сетевым upstream-адресом (поле
// server). local/fakeip/hosts адреса не имеют, dial-проверки к ним не
// применимы.
var dnsUpstreamTypes = map[string]bool{
	"udp":   true,
	"tls":   true,
	"https": true,
	"quic":  true,
	"h3":    true,
}

// dnsServerAddrIsDomain: server задан доменным именем (не IP-литерал).
func dnsServerAddrIsDomain(server string) bool {
	addr := strings.TrimSpace(server)
	if addr == "" {
		return false
	}
	addr = strings.TrimPrefix(addr, "[")
	addr = strings.TrimSuffix(addr, "]")
	_, err := netip.ParseAddr(addr)
	return err != nil
}

// computeDNSDialIssues — предупреждения о dial-ловушках DNS-серверов, которые
// проходят жёсткую валидацию, но ломаются в рантайме sing-box ≥1.14:
//
//   - server задан доменом без domain_resolver (и без
//     route.default_domain_resolver) — с 1.14 sing-box требует резолвер для
//     доменных адресов; исключение по документации — конфиг с единственным
//     DNS-сервером;
//   - domain_strategy — dial-поле удалено в 1.14 (deprecated с 1.12);
//   - detour вместе с другими dial-полями — «If enabled, all other fields
//     will be ignored»: domain_resolver при detour молча не применяется;
//   - domain_resolver / default_domain_resolver ссылается на несуществующий
//     DNS-сервер;
//   - цикл резолверов: доменный сервер, чья цепочка domain_resolver
//     возвращается в него самого, — dead lock резолвинга.
func computeDNSDialIssues(cfg *RouterConfig) []Issue {
	var issues []Issue

	serverTags := make(map[string]struct{}, len(cfg.DNS.Servers))
	for _, s := range cfg.DNS.Servers {
		serverTags[s.Tag] = struct{}{}
	}

	defaultResolver := ""
	if cfg.Route.DefaultDomainResolver != nil {
		defaultResolver = strings.TrimSpace(cfg.Route.DefaultDomainResolver.Server)
	}
	if defaultResolver != "" {
		if _, ok := serverTags[defaultResolver]; !ok {
			issues = append(issues, Issue{
				Severity: "warning",
				Kind:     "dns-domain-resolver",
				Tag:      defaultResolver,
				Message:  fmt.Sprintf("route.default_domain_resolver ссылается на несуществующий DNS-сервер %q", defaultResolver),
			})
		}
	}

	for _, s := range cfg.DNS.Servers {
		if s.Strategy != "" {
			issues = append(issues, Issue{
				Severity: "warning",
				Kind:     "dns-deprecated-field",
				Tag:      s.Tag,
				Message:  fmt.Sprintf("DNS-сервер %q использует domain_strategy — поле удалено в sing-box 1.14; задайте strategy внутри domain_resolver", s.Tag),
			})
		}

		if !dnsUpstreamTypes[s.Type] {
			continue
		}

		if s.Detour != "" && s.DomainResolver != nil {
			issues = append(issues, Issue{
				Severity: "warning",
				Kind:     "dns-detour-dial",
				Tag:      s.Tag,
				Message:  fmt.Sprintf("DNS-сервер %q: при заданном detour остальные dial-поля игнорируются — domain_resolver не будет применён", s.Tag),
			})
		}

		if !dnsServerAddrIsDomain(s.Server) {
			continue
		}

		resolver := ""
		if s.DomainResolver != nil {
			resolver = strings.TrimSpace(s.DomainResolver.Server)
		}

		if resolver != "" {
			if _, ok := serverTags[resolver]; !ok {
				issues = append(issues, Issue{
					Severity: "warning",
					Kind:     "dns-domain-resolver",
					Tag:      s.Tag,
					Message:  fmt.Sprintf("DNS-сервер %q: domain_resolver ссылается на несуществующий DNS-сервер %q", s.Tag, resolver),
				})
			}
		} else if s.Detour == "" && defaultResolver == "" && len(cfg.DNS.Servers) > 1 {
			issues = append(issues, Issue{
				Severity: "warning",
				Kind:     "dns-domain-resolver",
				Tag:      s.Tag,
				Message:  fmt.Sprintf("DNS-сервер %q задан доменом %q без domain_resolver — sing-box 1.14 требует резолвер (укажите IP-адрес, domain_resolver или route.default_domain_resolver)", s.Tag, s.Server),
			})
		}

		if cycle := dnsResolverCycle(cfg, s.Tag, defaultResolver); cycle != "" {
			issues = append(issues, Issue{
				Severity: "warning",
				Kind:     "dns-domain-resolver",
				Tag:      s.Tag,
				Message:  fmt.Sprintf("DNS-сервер %q: цикл domain_resolver (%s) — резолвинг доменного адреса зациклен", s.Tag, cycle),
			})
		}
	}

	return issues
}

// dnsResolverCycle идёт по цепочке «доменный сервер → его резолвер» начиная
// со start и возвращает строку цикла («a → b → a»), если цепочка возвращается
// в уже пройденный тег. Серверы с IP-адресом обрывают цепочку — им резолвер
// не нужен. Пустая строка — цикла нет.
func dnsResolverCycle(cfg *RouterConfig, start, defaultResolver string) string {
	byTag := make(map[string]*DNSServer, len(cfg.DNS.Servers))
	for i := range cfg.DNS.Servers {
		byTag[cfg.DNS.Servers[i].Tag] = &cfg.DNS.Servers[i]
	}

	visited := []string{start}
	seen := map[string]bool{start: true}
	cur := byTag[start]
	for cur != nil && dnsUpstreamTypes[cur.Type] && dnsServerAddrIsDomain(cur.Server) {
		next := defaultResolver
		// При detour dial-поля игнорируются — резолвер этого узла не участвует.
		if cur.Detour == "" && cur.DomainResolver != nil && strings.TrimSpace(cur.DomainResolver.Server) != "" {
			next = strings.TrimSpace(cur.DomainResolver.Server)
		}
		if next == "" {
			return ""
		}
		if seen[next] {
			return strings.Join(append(visited, next), " → ")
		}
		visited = append(visited, next)
		seen[next] = true
		cur = byTag[next]
	}
	return ""
}
