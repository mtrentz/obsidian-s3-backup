package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/joho/godotenv"
)

func compressDirectory(dirPath string, tarGzFileName string) error {
	tarGzFile, err := os.Create(tarGzFileName)
	if err != nil {
		return err
	}
	defer tarGzFile.Close()

	gzipWriter := gzip.NewWriter(tarGzFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

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

		tarEntryName := strings.ReplaceAll(relPath, string(filepath.Separator), "/")

		fileToTar, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer fileToTar.Close()

		header := &tar.Header{
			Name:    tarEntryName,
			Size:    info.Size(),
			Mode:    int64(info.Mode()),
			ModTime: info.ModTime(),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		_, err = io.Copy(tarWriter, fileToTar)
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

	tarGzName := time.Now().Format("2006-01-02_15-04-05") + ".tar.gz"
	err = compressDirectory(filePath, tarGzName)
	if err != nil {
		log.Fatal("Error compressing directory:", err)
	}
	tarGzPath := filepath.Join(".", tarGzName)

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

	// Setup BatchDeleteIterator to iterate through a list of objects.
	iter := s3manager.NewDeleteListIterator(svc, &s3.ListObjectsInput{
		Bucket: aws.String(bucketName),
	})

	// Traverse iterator deleting each object
	if err := s3manager.NewBatchDeleteWithClient(svc).Delete(aws.BackgroundContext(), iter); err != nil {
		log.Fatalf("Unable to delete objects from bucket %q, %v", bucketName, err)
	}

	fmt.Println("All files deleted successfully.")

	// Open the file
	file, err := os.Open(tarGzPath)
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
		Key:    aws.String(filepath.Base(tarGzPath)),
		Body:   bytes.NewReader(buffer),
	})
	if err != nil {
		log.Fatal("Error uploading file to S3:", err)
	}

	fmt.Println("File uploaded successfully.")

	// Delete the zip file
	err = os.Remove(tarGzPath)
}
