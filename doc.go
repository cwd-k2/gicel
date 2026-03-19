package gicel

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed docs/agent-guide
var agentGuideFS embed.FS

const agentGuideDir = "docs/agent-guide"

// DocTopics returns the list of available documentation topics.
// Subdirectory structure is flattened with "." separators:
// docs/agent-guide/features/records.md → "features.records"
func DocTopics() []string {
	var topics []string
	fs.WalkDir(agentGuideFS, agentGuideDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			return nil
		}
		// Convert path to dot-separated topic name.
		rel, _ := strings.CutPrefix(p, agentGuideDir+"/")
		topic := strings.TrimSuffix(rel, ".md")
		topic = strings.ReplaceAll(topic, "/", ".")
		topics = append(topics, topic)
		return nil
	})
	sort.Strings(topics)
	return topics
}

// Doc returns the agent guide content for the given topic.
// Pass "" or "index" for the table of contents (README).
// Dot-separated topics resolve to subdirectory paths:
// "features.records" → docs/agent-guide/features/records.md
// Returns empty string if the topic is not found.
func Doc(topic string) string {
	if topic == "" || topic == "index" {
		topic = "README"
	}
	// Convert dot-separated topic to file path.
	filePath := strings.ReplaceAll(topic, ".", "/") + ".md"
	data, err := agentGuideFS.ReadFile(path.Join(agentGuideDir, filePath))
	if err != nil {
		return ""
	}
	return string(data)
}

// DocDesc returns a short description for the given topic,
// extracted from the first "## " heading in the file.
// Returns empty string if no heading is found.
func DocDesc(topic string) string {
	content := Doc(topic)
	if content == "" {
		return ""
	}
	for _, line := range strings.SplitN(content, "\n", 20) {
		if title, ok := strings.CutPrefix(line, "## "); ok {
			// Strip leading "N. " numbering (e.g., "5. Effect System" → "Effect System").
			if i := strings.Index(title, ". "); i >= 0 && i <= 3 {
				title = title[i+2:]
			}
			return title
		}
	}
	return ""
}
