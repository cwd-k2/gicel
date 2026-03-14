package gicel

import (
	"embed"
	"path"
	"sort"
	"strings"
)

//go:embed docs/agent-guide/*.md
var agentGuideFS embed.FS

const agentGuideDir = "docs/agent-guide"

// DocTopics returns the list of available documentation topics.
func DocTopics() []string {
	entries, err := agentGuideFS.ReadDir(agentGuideDir)
	if err != nil {
		return nil
	}
	var topics []string
	for _, e := range entries {
		name := e.Name()
		if name == "README.md" {
			continue
		}
		topics = append(topics, strings.TrimSuffix(name, ".md"))
	}
	sort.Strings(topics)
	return topics
}

// Doc returns the agent guide content for the given topic.
// Pass "" or "index" for the table of contents (README).
// Returns empty string if the topic is not found.
func Doc(topic string) string {
	if topic == "" || topic == "index" {
		topic = "README"
	}
	data, err := agentGuideFS.ReadFile(path.Join(agentGuideDir, topic+".md"))
	if err != nil {
		return ""
	}
	return string(data)
}
