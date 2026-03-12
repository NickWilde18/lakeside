package knowledgeagent

import (
	"bytes"
	"encoding/gob"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResultRegisteredForGob(t *testing.T) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(struct {
		Output any
	}{
		Output: &Result{
			AgentName: "campus_it_kb",
			Success:   true,
			Message:   "ok",
			Sources: []Source{
				{
					KBID:     "kb-1",
					DocID:    "doc-1",
					NodeID:   "node-1",
					Filename: "manual.docx",
					Snippet:  "snippet",
					Score:    0.9,
				},
			},
		},
	})
	require.NoError(t, err)
}
