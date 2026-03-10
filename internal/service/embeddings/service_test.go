package embeddings

import (
	"context"
	"strings"
	"testing"

	cacheembed "github.com/cloudwego/eino-ext/components/embedding/cache"
)

func TestKeyGeneratorGenerate(t *testing.T) {
	generator := &keyGenerator{provider: "openrouter"}
	key := generator.Generate(context.Background(), "hello", cacheembed.GeneratorOption{Model: "qwen/qwen3-embedding-8b"})
	if !strings.HasPrefix(key, "openrouter:qwen_qwen3_embedding_8b:") {
		t.Fatalf("unexpected key prefix: %q", key)
	}
}

func TestSanitizeSegment(t *testing.T) {
	got := sanitizeSegment("qwen/qwen3-embedding-8b")
	if got != "qwen_qwen3_embedding_8b" {
		t.Fatalf("sanitizeSegment()=%q", got)
	}
}
