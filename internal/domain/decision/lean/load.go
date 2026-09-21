package lean

import (
	"fmt"
	"github.com/d3i-infra/adg/internal/domain/decision/madr"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// LoadDir walks modelPath and loads every NNNN-slug.md file as a lean Record,
// parsing frontmatter and body. Non-ADR markdown (e.g. README.md) is skipped.
// Records are returned sorted by ID. Record.Filename is relative to modelPath.
//
// IDs are a flat global NNNN across the whole model: the number is unique
// model-wide and index grouping comes from `category` frontmatter, not the
// directory layout. (The legacy per-subfolder AD#### scheme collided once the same
// number appeared in two folders; Validate hard-fails any duplicate ID — see
// duplicateIDIssues.)
func LoadDir(modelPath string) ([]Record, error) {
	var records []Record
	err := filepath.WalkDir(modelPath, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			return err
		}
		id, _, perr := madr.ParseFilename(path)
		if perr != nil {
			return nil // skip README.md and other non-ADR markdown
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fmText, body, splitErr := madr.SplitFile(raw)
		if splitErr != nil {
			return fmt.Errorf("%s: %w", path, splitErr)
		}
		fm, fmErr := madr.ParseFrontmatter(fmText)
		if fmErr != nil {
			return fmt.Errorf("%s: %w", path, fmErr)
		}
		rel, _ := filepath.Rel(modelPath, path)
		records = append(records, Record{
			ID:       id,
			Filename: rel,
			D:        madr.DecisionFromFrontmatter(fm),
			Body:     body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}
