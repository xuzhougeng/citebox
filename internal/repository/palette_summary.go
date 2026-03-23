package repository

import (
	"database/sql"
	"encoding/json"
	"strings"
)

func parsePaletteSummary(paletteID sql.NullInt64, paletteName, colorsJSON string) (*int64, string, []string, error) {
	if !paletteID.Valid {
		return nil, "", []string{}, nil
	}

	colors := []string{}
	if strings.TrimSpace(colorsJSON) != "" {
		if err := json.Unmarshal([]byte(colorsJSON), &colors); err != nil {
			return nil, "", nil, err
		}
	}
	if colors == nil {
		colors = []string{}
	}

	id := paletteID.Int64
	return &id, strings.TrimSpace(paletteName), colors, nil
}
