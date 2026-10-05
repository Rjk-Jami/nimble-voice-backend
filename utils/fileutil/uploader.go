package fileutil

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var AllowedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

const MaxFileSize = 5 * 1024 * 1024 // 5 MB

// SaveImage validates the uploaded file, generates a safe unique filename, saves it, and returns the accessible public path
func SaveImage(file *multipart.FileHeader, subFolder string) (string, error) {
	// 1. File size validation
	if file.Size > MaxFileSize {
		return "", errors.New("file size exceeds the 5MB limit")
	}

	// 2. Extension validation
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedImageExtensions[ext] {
		return "", fmt.Errorf("invalid file format: %s. Only JPG, PNG, WEBP, and GIF are allowed", ext)
	}

	// 3. Ensure target directory exists
	targetDir := filepath.Join("./uploads", subFolder)
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %v", err)
	}

	// 4. Generate unique filename (timestamp + uuid + original extension)
	uniqueFileName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.New().String()[:8], ext)
	destinationPath := filepath.Join(targetDir, uniqueFileName)

	// 5. Open and copy file to destination
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(destinationPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	// Relative public URL path for clients (e.g., /uploads/avatars/172810000_a1b2c3d4.png)
	publicURLPath := fmt.Sprintf("/uploads/%s/%s", subFolder, uniqueFileName)
	return publicURLPath, nil
}
