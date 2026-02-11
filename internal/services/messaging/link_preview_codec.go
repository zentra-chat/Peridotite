package messaging

import (
	"encoding/json"

	"github.com/zentra/peridotite/internal/models"
)

func EncodeLinkPreviews(previews []models.LinkPreview) []byte {
	payload, err := json.Marshal(previews)
	if err != nil {
		return []byte("[]")
	}
	return payload
}

func DecodeLinkPreviews(raw []byte) []models.LinkPreview {
	if len(raw) == 0 {
		return nil
	}

	var previews []models.LinkPreview
	if err := json.Unmarshal(raw, &previews); err != nil {
		return nil
	}

	return previews
}
