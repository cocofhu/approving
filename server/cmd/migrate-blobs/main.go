// Command migrate-blobs externalizes legacy inline base64 PromptImage.data
// fields into the configured BlobStore and rewrites DB JSON to blob:{id} refs.
//
// Usage (from server/):
//
//	go run ./cmd/migrate-blobs [config.yaml]
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cocofhu/approving/internal/blob"
	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/logging"
	"github.com/cocofhu/approving/internal/models"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func main() {
	logging.Setup()
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	if err := config.Load(cfgPath); err != nil {
		log.Fatal().Err(err).Str("path", cfgPath).Msg("load config failed")
	}
	cfg := config.GetConfig()
	store, err := blob.NewFromConfig(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("blob store init failed")
	}
	db, err := database.Open(cfg.Database)
	if err != nil {
		log.Fatal().Err(err).Msg("open database failed")
	}
	ctx := context.Background()
	n, err := migrateAll(ctx, db, store)
	if err != nil {
		log.Fatal().Err(err).Msg("migrate failed")
	}
	log.Info().Int("updated_rows", n).Msg("blob migration complete")
}

func migrateAll(ctx context.Context, db *gorm.DB, store blob.Store) (int, error) {
	updated := 0
	var msgs []models.ChatMessage
	if err := db.Find(&msgs).Error; err != nil {
		return 0, err
	}
	for i := range msgs {
		imgs, changed, err := migrateImages(ctx, store, msgs[i].Images)
		if err != nil {
			return updated, fmt.Errorf("chat_messages %s: %w", msgs[i].ID, err)
		}
		if !changed {
			continue
		}
		msgs[i].Images = imgs
		if err := db.Model(&msgs[i]).Select("Images").Updates(&msgs[i]).Error; err != nil {
			return updated, err
		}
		updated++
	}

	var issues []models.PreviewIssue
	if err := db.Find(&issues).Error; err != nil {
		return updated, err
	}
	for i := range issues {
		imgs, changed, err := migrateImages(ctx, store, issues[i].Images)
		if err != nil {
			return updated, fmt.Errorf("preview_issues %s: %w", issues[i].ID, err)
		}
		if !changed {
			continue
		}
		issues[i].Images = imgs
		if err := db.Model(&issues[i]).Select("Images").Updates(&issues[i]).Error; err != nil {
			return updated, err
		}
		updated++
	}

	var convs []models.ReactConversation
	if err := db.Find(&convs).Error; err != nil {
		return updated, err
	}
	for i := range convs {
		changed := false
		for j := range convs[i].Messages {
			imgs, ch, err := migrateImages(ctx, store, convs[i].Messages[j].Images)
			if err != nil {
				return updated, fmt.Errorf("react_conversations %d turn %d: %w", convs[i].ID, j, err)
			}
			if ch {
				convs[i].Messages[j].Images = imgs
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := db.Model(&convs[i]).Select("Messages").Updates(&convs[i]).Error; err != nil {
			return updated, err
		}
		updated++
	}

	var vars []models.RunVariable
	if err := db.Find(&vars).Error; err != nil {
		return updated, err
	}
	for i := range vars {
		nv, changed, err := migrateValue(ctx, store, vars[i].Value)
		if err != nil {
			return updated, fmt.Errorf("run_variables %s/%s: %w", vars[i].RunID, vars[i].Name, err)
		}
		if !changed {
			continue
		}
		vars[i].Value = nv
		if err := db.Save(&vars[i]).Error; err != nil {
			return updated, err
		}
		updated++
	}

	var runs []models.Run
	if err := db.Select("id", "inputs").Find(&runs).Error; err != nil {
		return updated, err
	}
	for i := range runs {
		if len(runs[i].Inputs) == 0 {
			continue
		}
		raw, _ := json.Marshal(runs[i].Inputs)
		var inputs map[string]any
		if err := json.Unmarshal(raw, &inputs); err != nil {
			continue
		}
		changed := false
		for k, v := range inputs {
			nv, ch, err := migrateValue(ctx, store, v)
			if err != nil {
				return updated, fmt.Errorf("runs %s inputs.%s: %w", runs[i].ID, k, err)
			}
			if ch {
				inputs[k] = nv
				changed = true
			}
		}
		if !changed {
			continue
		}
		runs[i].Inputs = inputs
		if err := db.Model(&runs[i]).Select("Inputs").Updates(&runs[i]).Error; err != nil {
			return updated, err
		}
		updated++
	}
	return updated, nil
}

func migrateImages(ctx context.Context, store blob.Store, images []models.PromptImage) ([]models.PromptImage, bool, error) {
	if len(images) == 0 {
		return images, false, nil
	}
	need := false
	for _, im := range images {
		if im.Data != "" && im.Ref == "" {
			need = true
			break
		}
	}
	if !need {
		return images, false, nil
	}
	out, err := blob.IngestPromptImages(ctx, store, images)
	return out, true, err
}

func migrateValue(ctx context.Context, store blob.Store, v any) (any, bool, error) {
	ct := models.AsCompositeText(v)
	if ct == nil || len(ct.Images) == 0 {
		return v, false, nil
	}
	imgs, changed, err := migrateImages(ctx, store, ct.Images)
	if err != nil || !changed {
		return v, changed, err
	}
	return map[string]any{
		"text": ct.Text,
		"images": func() []any {
			out := make([]any, len(imgs))
			for i, im := range imgs {
				m := map[string]any{"mimeType": im.MimeType, "ref": im.Ref}
				if im.Name != "" {
					m["name"] = im.Name
				}
				if im.SizeBytes > 0 {
					m["sizeBytes"] = im.SizeBytes
				}
				out[i] = m
			}
			return out
		}(),
	}, true, nil
}
