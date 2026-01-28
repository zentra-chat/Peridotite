package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/nfnt/resize"
	"github.com/zentra/peridotite/internal/models"
	"github.com/zentra/peridotite/internal/services/community"
)

var (
	ErrFileTooLarge       = errors.New("file too large")
	ErrInvalidFileType    = errors.New("invalid file type")
	ErrUploadFailed       = errors.New("upload failed")
	ErrAttachmentNotFound = errors.New("attachment not found")
)

// File size limits
const (
	MaxImageSize       = 10 * 1024 * 1024  // 10MB
	MaxVideoSize       = 100 * 1024 * 1024 // 100MB
	MaxFileSize        = 50 * 1024 * 1024  // 50MB
	MaxAvatarSize      = 5 * 1024 * 1024   // 5MB
	ThumbnailMaxWidth  = 400
	ThumbnailMaxHeight = 300
)

// Allowed MIME types
var (
	AllowedImageTypes = map[string]bool{
		"image/jpeg": true,
		"image/png":  true,
		"image/gif":  true,
		"image/webp": true,
	}
	AllowedVideoTypes = map[string]bool{
		"video/mp4":       true,
		"video/webm":      true,
		"video/quicktime": true,
	}
	AllowedAudioTypes = map[string]bool{
		"audio/mpeg": true,
		"audio/ogg":  true,
		"audio/wav":  true,
		"audio/webm": true,
	}
	AllowedDocumentTypes = map[string]bool{
		"application/pdf":             true,
		"text/plain":                  true,
		"application/zip":             true,
		"application/x-rar":           true,
		"application/x-7z-compressed": true,
	}
)

type Service struct {
	db               *pgxpool.Pool
	minio            *minio.Client
	bucketName       string
	cdnBaseURL       string
	communityService *community.Service
}

func NewService(db *pgxpool.Pool, minioClient *minio.Client, bucketName, cdnBaseURL string, communityService *community.Service) *Service {
	return &Service{
		db:               db,
		minio:            minioClient,
		bucketName:       bucketName,
		cdnBaseURL:       cdnBaseURL,
		communityService: communityService,
	}
}

type UploadResult struct {
	ID           uuid.UUID `json:"id"`
	Filename     string    `json:"filename"`
	ContentType  string    `json:"mimeType"`
	Size         int64     `json:"size"`
	URL          string    `json:"url"`
	ThumbnailURL *string   `json:"thumbnailUrl,omitempty"`
}

// UploadAttachment handles file uploads for message attachments
func (s *Service) UploadAttachment(ctx context.Context, userID uuid.UUID, file multipart.File, header *multipart.FileHeader) (*UploadResult, error) {
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Validate file type and size
	maxSize := s.getMaxSizeForType(contentType)
	if header.Size > maxSize {
		return nil, ErrFileTooLarge
	}

	if !s.isAllowedType(contentType) {
		return nil, ErrInvalidFileType
	}

	// Read file content
	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Generate unique filename
	ext := filepath.Ext(header.Filename)
	attachmentID := uuid.New()
	objectName := fmt.Sprintf("attachments/%s/%s%s", userID.String(), attachmentID.String(), ext)

	// Upload to MinIO
	_, err = s.minio.PutObject(ctx, s.bucketName, objectName, bytes.NewReader(fileData), int64(len(fileData)),
		minio.PutObjectOptions{
			ContentType: contentType,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	fileURL := fmt.Sprintf("%s/%s/%s", s.cdnBaseURL, s.bucketName, objectName)

	// Generate thumbnail for images
	var thumbnailURL *string
	if AllowedImageTypes[contentType] {
		thumbURL, err := s.generateThumbnail(ctx, fileData, attachmentID, userID, ext)
		if err == nil {
			thumbnailURL = &thumbURL
		}
	}

	// Store in database
	contentTypePtr := &contentType
	attachment := &models.MessageAttachment{
		ID:           attachmentID,
		UploaderID:   userID,
		Filename:     header.Filename,
		ContentType:  contentTypePtr,
		FileSize:     header.Size,
		FileURL:      fileURL,
		ThumbnailURL: thumbnailURL,
		CreatedAt:    time.Now(),
	}

	query := `
		INSERT INTO message_attachments (id, uploader_id, filename, content_type, file_size, file_url, thumbnail_url, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = s.db.Exec(ctx, query,
		attachment.ID, attachment.UploaderID, attachment.Filename,
		attachment.ContentType, attachment.FileSize, attachment.FileURL,
		attachment.ThumbnailURL, attachment.CreatedAt,
	)
	if err != nil {
		// Cleanup uploaded file
		s.minio.RemoveObject(ctx, s.bucketName, objectName, minio.RemoveObjectOptions{})
		return nil, fmt.Errorf("failed to save attachment record: %w", err)
	}

	return &UploadResult{
		ID:           attachment.ID,
		Filename:     attachment.Filename,
		ContentType:  *attachment.ContentType,
		Size:         attachment.FileSize,
		URL:          attachment.FileURL,
		ThumbnailURL: attachment.ThumbnailURL,
	}, nil
}

// UploadAvatar handles avatar uploads for users/communities
func (s *Service) UploadAvatar(ctx context.Context, ownerID uuid.UUID, ownerType string, file multipart.File, header *multipart.FileHeader) (string, error) {
	contentType := header.Header.Get("Content-Type")
	if !AllowedImageTypes[contentType] {
		return "", ErrInvalidFileType
	}

	if header.Size > MaxAvatarSize {
		return "", ErrFileTooLarge
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Process image - resize if needed
	processedData, err := s.processAvatar(fileData)
	if err != nil {
		return "", fmt.Errorf("failed to process avatar: %w", err)
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}

	objectName := fmt.Sprintf("avatars/%s/%s%s", ownerType, ownerID.String(), ext)

	// Upload to MinIO
	_, err = s.minio.PutObject(ctx, s.bucketName, objectName, bytes.NewReader(processedData), int64(len(processedData)),
		minio.PutObjectOptions{
			ContentType: "image/jpeg",
		})
	if err != nil {
		return "", fmt.Errorf("failed to upload avatar: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", s.cdnBaseURL, s.bucketName, objectName), nil
}

// UploadCommunityAsset handles community banner/icon uploads
// assetType should be "banner" or "icon"
func (s *Service) UploadCommunityAsset(ctx context.Context, communityID uuid.UUID, assetType string, file multipart.File, header *multipart.FileHeader) (string, error) {
	contentType := header.Header.Get("Content-Type")
	if !AllowedImageTypes[contentType] {
		return "", ErrInvalidFileType
	}

	if header.Size > MaxImageSize {
		return "", ErrFileTooLarge
	}

	fileData, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	ext := filepath.Ext(header.Filename)
	objectName := fmt.Sprintf("communities/%s/%s%s", communityID.String(), assetType, ext)

	_, err = s.minio.PutObject(ctx, s.bucketName, objectName, bytes.NewReader(fileData), int64(len(fileData)),
		minio.PutObjectOptions{
			ContentType: contentType,
		})
	if err != nil {
		return "", fmt.Errorf("failed to upload asset: %w", err)
	}

	return fmt.Sprintf("%s/%s/%s", s.cdnBaseURL, s.bucketName, objectName), nil
}

// GetAttachment retrieves attachment metadata
func (s *Service) GetAttachment(ctx context.Context, attachmentID uuid.UUID) (*models.MessageAttachment, error) {
	var a models.MessageAttachment
	query := `
		SELECT id, message_id, message_created_at, uploader_id, filename, file_url, file_size, content_type, thumbnail_url, width, height, created_at
		FROM message_attachments
		WHERE id = $1`

	err := s.db.QueryRow(ctx, query, attachmentID).Scan(
		&a.ID, &a.MessageID, &a.MessageCreatedAt, &a.UploaderID, &a.Filename, &a.FileURL, &a.FileSize,
		&a.ContentType, &a.ThumbnailURL, &a.Width, &a.Height, &a.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAttachmentNotFound
		}
		return nil, err
	}

	return &a, nil
}

// DeleteAttachment removes an attachment
func (s *Service) DeleteAttachment(ctx context.Context, attachmentID, userID uuid.UUID) error {
	// Get attachment to verify ownership and get URL for deletion
	attachment, err := s.GetAttachment(ctx, attachmentID)
	if err != nil {
		return err
	}

	if attachment.UploaderID != userID {
		return errors.New("not attachment owner")
	}

	// Delete from database
	_, err = s.db.Exec(ctx, `DELETE FROM message_attachments WHERE id = $1`, attachmentID)
	if err != nil {
		return err
	}

	// Delete from MinIO
	objectName := strings.TrimPrefix(attachment.FileURL, fmt.Sprintf("%s/%s/", s.cdnBaseURL, s.bucketName))
	s.minio.RemoveObject(ctx, s.bucketName, objectName, minio.RemoveObjectOptions{})

	// Delete thumbnail if exists
	if attachment.ThumbnailURL != nil {
		thumbObjectName := strings.TrimPrefix(*attachment.ThumbnailURL, fmt.Sprintf("%s/%s/", s.cdnBaseURL, s.bucketName))
		s.minio.RemoveObject(ctx, s.bucketName, thumbObjectName, minio.RemoveObjectOptions{})
	}

	return nil
}

// GetPresignedURL generates a presigned URL for direct download
func (s *Service) GetPresignedURL(ctx context.Context, attachmentID uuid.UUID, expiry time.Duration) (string, error) {
	attachment, err := s.GetAttachment(ctx, attachmentID)
	if err != nil {
		return "", err
	}

	objectName := strings.TrimPrefix(attachment.FileURL, fmt.Sprintf("%s/%s/", s.cdnBaseURL, s.bucketName))

	presignedURL, err := s.minio.PresignedGetObject(ctx, s.bucketName, objectName, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	return presignedURL.String(), nil
}

// Helper functions
func (s *Service) getMaxSizeForType(contentType string) int64 {
	if AllowedImageTypes[contentType] {
		return MaxImageSize
	}
	if AllowedVideoTypes[contentType] {
		return MaxVideoSize
	}
	return MaxFileSize
}

func (s *Service) isAllowedType(contentType string) bool {
	return AllowedImageTypes[contentType] ||
		AllowedVideoTypes[contentType] ||
		AllowedAudioTypes[contentType] ||
		AllowedDocumentTypes[contentType]
}

// generateThumbnail creates a thumbnail for image attachments
// Currnetly, only JPEG thumbnails are generated.
// I need to modify this later to support PNG's with transparency.
// For now, this will do.
func (s *Service) generateThumbnail(ctx context.Context, imageData []byte, attachmentID, userID uuid.UUID, ext string) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return "", err
	}

	// Resize maintaining aspect ratio
	thumb := resize.Thumbnail(ThumbnailMaxWidth, ThumbnailMaxHeight, img, resize.Lanczos3)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: 80}); err != nil {
		return "", err
	}

	thumbObjectName := fmt.Sprintf("attachments/%s/thumbs/%s_thumb.jpg", userID.String(), attachmentID.String())

	_, err = s.minio.PutObject(ctx, s.bucketName, thumbObjectName, &buf, int64(buf.Len()),
		minio.PutObjectOptions{
			ContentType: "image/jpeg",
		})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s/%s", s.cdnBaseURL, s.bucketName, thumbObjectName), nil
}

func (s *Service) processAvatar(imageData []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, err
	}

	// Resize to 256x256 max while maintaining aspect ratio
	// I should allow larger sizes in the future, but for now this is fine.
	// Nobody is really using it right now, and this should be recreated by the time
	// it ever hits production anyways, so optimization isn't a huge concern.
	resized := resize.Thumbnail(256, 256, img, resize.Lanczos3)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *Service) RequirePermission(ctx context.Context, communityID, userID uuid.UUID, permission int64) error {
	return s.communityService.RequirePermission(ctx, communityID, userID, permission)
}
