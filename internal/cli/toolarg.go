package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/MrMirhan/a2migrate/internal/migrate"
	"github.com/MrMirhan/a2migrate/internal/tools"
)

// Tool ids the migration pipelines are implemented for. Every other
// registry entry resolves as an argument but has no pipeline yet.
const (
	toolClaudeCode tools.ID = "claude_code"
	toolOpenCode   tools.ID = "opencode"
)

// toolAliases are typing shortcuts. Registry ids and their kebab-case
// spelling always resolve; this map only adds extras.
var toolAliases = map[string]tools.ID{
	"cc": toolClaudeCode,
	"oc": toolOpenCode,
}

// toolArg renders a registry id the way it is written on the command
// line: kebab-case, because underscores are awkward to type.
func toolArg(id tools.ID) string {
	return strings.ReplaceAll(string(id), "_", "-")
}

// resolveTool maps a command-line argument to a registry tool.
func resolveTool(arg string) (tools.Tool, error) {
	key := strings.ToLower(strings.TrimSpace(arg))
	if id, ok := toolAliases[key]; ok {
		if t, ok := tools.Get(id); ok {
			return t, nil
		}
	}
	want := strings.ReplaceAll(key, "-", "_")
	for _, t := range tools.All() {
		if strings.EqualFold(string(t.ID), want) {
			return t, nil
		}
	}
	return tools.Tool{}, fmt.Errorf("unknown tool %q (known: %s)", arg, strings.Join(toolArgs(), ", "))
}

// toolArgs lists every registered tool as written on the command line.
func toolArgs() []string {
	ids := tools.IDs()
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, toolArg(id))
	}
	sort.Strings(out)
	return out
}

// domainSpec maps a command-line domain name onto the registry
// capability that gates it and, for artifact domains, the
// ArtifactsMigrator domain that implements it. Sessions have their own
// orchestrator, so their art field stays empty.
type domainSpec struct {
	name string
	cap  tools.Capability
	art  migrate.Domain
}

var cliDomains = []domainSpec{
	{"sessions", tools.CapSessions, ""},
	{"skills", tools.CapSkills, migrate.DomainSkills},
	{"commands", tools.CapCommands, migrate.DomainCommands},
	{"agents", tools.CapAgents, migrate.DomainAgents},
	{"rules", tools.CapRules, migrate.DomainRules},
	{"mcp", tools.CapMCP, migrate.DomainMCP},
	{"system", tools.CapSystemPrompt, migrate.DomainSystem},
}

func domainByName(name string) (domainSpec, bool) {
	for _, d := range cliDomains {
		if d.name == strings.ToLower(strings.TrimSpace(name)) {
			return d, true
		}
	}
	return domainSpec{}, false
}

// sharedDomains returns the domains both tools declare support for, in
// cliDomains order.
func sharedDomains(ts ...tools.Tool) []domainSpec {
	out := make([]domainSpec, 0, len(cliDomains))
	for _, d := range cliDomains {
		ok := true
		for _, t := range ts {
			if !t.Has(d.cap) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, d)
		}
	}
	return out
}

func domainNames(specs []domainSpec) []string {
	out := make([]string, 0, len(specs))
	for _, d := range specs {
		out = append(out, d.name)
	}
	return out
}

// resolveDomains validates the requested domain arguments against what
// every participating tool supports. No arguments means every shared
// domain.
func resolveDomains(args []string, ts ...tools.Tool) ([]domainSpec, error) {
	shared := sharedDomains(ts...)
	if len(args) == 0 {
		if len(shared) == 0 {
			return nil, fmt.Errorf("no domain is supported by all of: %s", strings.Join(toolDisplayNames(ts), ", "))
		}
		return shared, nil
	}

	seen := make(map[string]bool, len(args))
	out := make([]domainSpec, 0, len(args))
	for _, a := range args {
		d, ok := domainByName(a)
		if !ok {
			return nil, fmt.Errorf("unknown domain %q (known: %s)", a, strings.Join(domainNames(cliDomains), ", "))
		}
		for _, t := range ts {
			if !t.Has(d.cap) {
				return nil, fmt.Errorf("%s does not support %s", t.DisplayName, d.name)
			}
		}
		if seen[d.name] {
			continue
		}
		seen[d.name] = true
		out = append(out, d)
	}
	return out, nil
}

func toolDisplayNames(ts []tools.Tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.DisplayName)
	}
	return out
}

// artifactDomains filters out sessions, which ArtifactsMigrator does not
// handle.
func artifactDomains(specs []domainSpec) []migrate.Domain {
	out := make([]migrate.Domain, 0, len(specs))
	for _, d := range specs {
		if d.art != "" {
			out = append(out, d.art)
		}
	}
	return out
}

func hasDomain(specs []domainSpec, name string) bool {
	for _, d := range specs {
		if d.name == name {
			return true
		}
	}
	return false
}

// completeMigrateArgs completes `migrate <from> <to> [domain...]`. The
// domain positions offer only what both tools support, so an unsupported
// combination cannot be tab-completed into existence.
func completeMigrateArgs(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return toolArgs(), cobra.ShellCompDirectiveNoFileComp
	case 1:
		from, err := resolveTool(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		out := make([]string, 0, len(cliDomains))
		for _, name := range toolArgs() {
			if name != toolArg(from.ID) {
				out = append(out, name)
			}
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	default:
		from, err := resolveTool(args[0])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		to, err := resolveTool(args[1])
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return remaining(domainNames(sharedDomains(from, to)), args[2:]), cobra.ShellCompDirectiveNoFileComp
	}
}

// completeToolThenDomain completes `<verb> <tool> [domain]`.
func completeToolThenDomain(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return toolArgs(), cobra.ShellCompDirectiveNoFileComp
	}
	t, err := resolveTool(args[0])
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return remaining(domainNames(sharedDomains(t)), args[1:]), cobra.ShellCompDirectiveNoFileComp
}

// completeToolOnly completes a single tool positional.
func completeToolOnly(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return toolArgs(), cobra.ShellCompDirectiveNoFileComp
}

func remaining(all, used []string) []string {
	seen := make(map[string]bool, len(used))
	for _, u := range used {
		seen[u] = true
	}
	out := make([]string, 0, len(all))
	for _, a := range all {
		if !seen[a] {
			out = append(out, a)
		}
	}
	return out
}
