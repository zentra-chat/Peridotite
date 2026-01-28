package storage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
	"github.com/zentra/peridotite/config"
)

var MinIOClient *minio.Client

func ConnectMinIO(cfg *config.Config) (*minio.Client, error) {
	client, err := minio.New(cfg.Storage.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.Storage.AccessKey, cfg.Storage.SecretKey, ""),
		Secure: cfg.Storage.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Verify connection by checking if buckets exist
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	buckets := []string{
		cfg.Storage.BucketAttachments,
		cfg.Storage.BucketAvatars,
		cfg.Storage.BucketCommunity,
	}

	for _, bucket := range buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return nil, fmt.Errorf("failed to check bucket %s: %w", bucket, err)
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
				return nil, fmt.Errorf("failed to create bucket %s: %w", bucket, err)
			}
			log.Info().Str("bucket", bucket).Msg("Created MinIO bucket")
		}

		// Set public-read policy for all buckets by default for CDN access
		policy := fmt.Sprintf(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Action": ["s3:GetBucketLocation", "s3:ListBucket"],
					"Effect": "Allow",
					"Principal": "*",
					"Resource": ["arn:aws:s3:::%s"]
				},
				{
					"Action": ["s3:GetObject"],
					"Effect": "Allow",
					"Principal": "*",
					"Resource": ["arn:aws:s3:::%s/*"]
				}
			]
		}`, bucket, bucket)
		err = client.SetBucketPolicy(ctx, bucket, policy)
		if err != nil {
			log.Warn().Err(err).Str("bucket", bucket).Msg("Failed to set public policy on bucket")
		}
	}

	MinIOClient = client
	log.Info().Str("endpoint", cfg.Storage.Endpoint).Msg("Connected to MinIO")

	return client, nil
}

type UploadResult struct {
	URL         string
	Filename    string
	Size        int64
	ContentType string
	Width       int
	Height      int
}

// UploadFile uploads a file to the specified bucket
func UploadFile(ctx context.Context, bucket, filename string, reader io.Reader, size int64, contentType string) (*UploadResult, error) {
	// Generate unique filename to prevent collisions
	ext := filepath.Ext(filename)
	uniqueName := fmt.Sprintf("%s%s", uuid.New().String(), ext)

	// Organize by date
	now := time.Now()
	objectName := fmt.Sprintf("%d/%02d/%02d/%s", now.Year(), now.Month(), now.Day(), uniqueName)

	_, err := MinIOClient.PutObject(ctx, bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	cfg := config.AppConfig
	var fileURL string
	if cfg.Storage.UseSSL {
		fileURL = fmt.Sprintf("https://%s/%s/%s", cfg.Storage.Endpoint, bucket, objectName)
	} else {
		fileURL = fmt.Sprintf("http://%s/%s/%s", cfg.Storage.Endpoint, bucket, objectName)
	}

	return &UploadResult{
		URL:         fileURL,
		Filename:    filename,
		Size:        size,
		ContentType: contentType,
	}, nil
}

// UploadImage uploads an image with dimension extraction
func UploadImage(ctx context.Context, bucket, filename string, data []byte, contentType string) (*UploadResult, error) {
	reader := bytes.NewReader(data)

	// Get image dimensions
	imgConfig, _, err := image.DecodeConfig(reader)
	width, height := 0, 0
	if err == nil {
		width = imgConfig.Width
		height = imgConfig.Height
	}

	// Reset reader for upload
	reader.Seek(0, io.SeekStart)

	result, err := UploadFile(ctx, bucket, filename, reader, int64(len(data)), contentType)
	if err != nil {
		return nil, err
	}

	result.Width = width
	result.Height = height
	return result, nil
}

// DeleteFile removes a file from storage
func DeleteFile(ctx context.Context, bucket, objectName string) error {
	return MinIOClient.RemoveObject(ctx, bucket, objectName, minio.RemoveObjectOptions{})
}

// DeleteFileByURL removes a file using its full URL
func DeleteFileByURL(ctx context.Context, fileURL string) error {
	parsed, err := url.Parse(fileURL)
	if err != nil {
		return fmt.Errorf("invalid file URL: %w", err)
	}

	// Extract bucket and object from path (format: /bucket/object/path)
	parts := strings.SplitN(strings.TrimPrefix(parsed.Path, "/"), "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid file URL format")
	}

	return DeleteFile(ctx, parts[0], parts[1])
}

// GetPresignedURL generates a temporary URL for private files
func GetPresignedURL(ctx context.Context, bucket, objectName string, expiry time.Duration) (string, error) {
	presignedURL, err := MinIOClient.PresignedGetObject(ctx, bucket, objectName, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

// IsImageContentType checks if the content type is a supported image format
func IsImageContentType(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// GetContentTypeFromFilename returns content type based on file extension
func GetContentTypeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}
