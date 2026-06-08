package geo

import (
	"bufio"
	"io"
	"strings"
)

type RobotsState string

const (
	RobotsAllowed    RobotsState = "allowed"
	RobotsDisallowed RobotsState = "disallowed"
	RobotsUnknown    RobotsState = "unknown"
)

type robotsRule struct {
	allow   bool
	prefix  string
	ordinal int
}

type RobotsRules struct {
	byAgent map[string][]robotsRule
}

func ParseRobots(r io.Reader) RobotsRules {
	rules := RobotsRules{byAgent: map[string][]robotsRule{}}
	scanner := bufio.NewScanner(r)
	var agents []string
	seenDirective := false
	ordinal := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		key, value, ok := splitRobotsDirective(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "user-agent":
			if seenDirective || len(agents) == 0 {
				agents = nil
				seenDirective = false
			}
			agents = append(agents, normalizeAgent(value))
		case "allow", "disallow":
			if len(agents) == 0 {
				continue
			}
			seenDirective = true
			if strings.ToLower(key) == "disallow" && strings.TrimSpace(value) == "" {
				continue
			}
			ordinal++
			rule := robotsRule{allow: strings.ToLower(key) == "allow", prefix: strings.TrimSpace(value), ordinal: ordinal}
			for _, agent := range agents {
				rules.byAgent[agent] = append(rules.byAgent[agent], rule)
			}
		}
	}
	return rules
}

func (r RobotsRules) StateFor(agent, path string) RobotsState {
	rules := r.byAgent[normalizeAgent(agent)]
	if len(rules) == 0 {
		rules = r.byAgent["*"]
	}
	if len(rules) == 0 {
		return RobotsUnknown
	}
	var matched *robotsRule
	for i := range rules {
		rule := rules[i]
		if rule.prefix == "" || !strings.HasPrefix(path, rule.prefix) {
			continue
		}
		if matched == nil || len(rule.prefix) > len(matched.prefix) || (len(rule.prefix) == len(matched.prefix) && rule.ordinal > matched.ordinal) {
			matched = &rules[i]
		}
	}
	if matched == nil {
		return RobotsAllowed
	}
	if matched.allow {
		return RobotsAllowed
	}
	return RobotsDisallowed
}

func splitRobotsDirective(line string) (string, string, bool) {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+1:]), true
}

func normalizeAgent(agent string) string {
	return strings.ToLower(strings.TrimSpace(agent))
}
