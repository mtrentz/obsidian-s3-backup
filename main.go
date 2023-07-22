package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/joho/godotenv"
)

func zipDirectory(dirPath string, zipFileName string) error {
	zipFile, err := os.Create(zipFileName)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(dirPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		relPath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			return err
		}

		zipEntryName := strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileToZip, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer fileToZip.Close()

		zipFile, err := zipWriter.Create(zipEntryName)
		if err != nil {
			return err
		}

		_, err = io.Copy(zipFile, fileToZip)
		if err != nil {
			return err
		}

		return nil
	})
}

func resolvePath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal("Error getting user home directory:", err)
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return path
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	bucketName := os.Getenv("AWS_S3_BUCKET_NAME")
	filePath := resolvePath(os.Getenv("OBSIDIAN_VAULT_PATH"))

	zipName := time.Now().Format("2006-01-02_15-04-05") + ".zip"
	err = zipDirectory(filePath, zipName)
	if err != nil {
		log.Fatal("Error zipping directory:", err)
	}
	zipPath := filepath.Join(".", zipName)

	// Create a new AWS session based on the environment variables
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
		Credentials: credentials.NewStaticCredentials(
			os.Getenv("AWS_ACCESS_KEY"),
			os.Getenv("AWS_SECRET_KEY"),
			"",
		),
	}))

	// Create an S3 service client
	svc := s3.New(sess)

	// Open the file
	file, err := os.Open(zipPath)
	if err != nil {
		log.Fatal("Error opening file:", err)
	}
	defer file.Close()

	// Get the file size and prepare a buffer to read the file
	fileInfo, _ := file.Stat()
	size := fileInfo.Size()
	buffer := make([]byte, size)
	file.Read(buffer)

	// Upload the file to S3
	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(filepath.Base(zipPath)),
		Body:   bytes.NewReader(buffer),
	})
	if err != nil {
		log.Fatal("Error uploading file to S3:", err)
	}

	fmt.Println("File uploaded successfully.")

	// Delete the zip file
	err = os.Remove(zipPath)
}
