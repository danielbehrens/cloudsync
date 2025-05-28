package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/minio/minio-go/v7"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	timeTolerance = 500 * time.Millisecond
	eventCooldown = 1 * time.Second
)

var (
	lastEventTime = make(map[string]time.Time)
)

func main() {
	client, watcher := configure()
	defer watcher.Close()
	ctx := context.Background()

	log.Print("starting cloudsync")
	defer log.Print("closing cloudsync")
	log.Printf("Watching %s for changes...", watchPath)
	log.Println("Performing initial sync...")
	localAndCloudSync(ctx, client)
	log.Println("Initial sync complete.")

	for {
		select {
		case event := <-watcher.Events:
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if processName != "" && isProcessRunning(processName) {
					log.Printf("Process '%s' is running. Sync paused.", processName)
					continue
				}

				if shouldSyncFile(event.Name) && filepath.Dir(event.Name) == filepath.Clean(watchPath) {
					now := time.Now().UTC()
					last, seen := lastEventTime[event.Name]
					if !seen || now.Sub(last) > eventCooldown {
						log.Printf("Detected change: %s", event.Name)
						lastEventTime[event.Name] = now
						checkCloudAndSync(ctx, client, event.Name)
					}
				}
			}
		case err := <-watcher.Errors:
			log.Println("Watcher error:", err)
		case <-time.After(time.Second * 10):
			if processName != "" && !isProcessRunning(processName) {
				localAndCloudSync(ctx, client)
			}
		case <-ctx.Done():
			return
		}
	}
}

func isProcessRunning(name string) bool {
	processes, err := process.Processes()
	if err != nil {
		log.Printf("Error listing processes: %v", err)
		return false
	}

	name = strings.ToLower(name)

	for _, p := range processes {
		exeName, err := p.Name()
		if err == nil && strings.Contains(strings.ToLower(exeName), name) {
			return true
		}
	}
	return false
}

func shouldSyncFile(filePath string) bool {
	name := filepath.Base(filePath)
	return strings.HasSuffix(name, ".sav") && name != "EnhancedInputUserSettings.sav"
}

func localAndCloudSync(ctx context.Context, client *minio.Client) {
	// Upload local files if newer
	entries, err := os.ReadDir(watchPath)
	if err != nil {
		log.Printf("Failed to read watch directory: %v", err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(watchPath, entry.Name())
		if shouldSyncFile(path) {
			checkCloudAndSync(ctx, client, path)
		}
	}

	// Download newer files from cloud
	syncFromCloud(ctx, client)
}

func syncFromCloud(ctx context.Context, client *minio.Client) {
	objectCh := client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{Recursive: true})

	for object := range objectCh {
		if object.Err != nil {
			log.Printf("Error listing object: %v", object.Err)
			continue
		}

		objectName := object.Key
		if !shouldSyncFile(objectName) {
			continue
		}

		localPath := filepath.Join(watchPath, filepath.Base(objectName))
		localInfo, err := os.Stat(localPath)
		if err != nil && !os.IsNotExist(err) {
			log.Printf("Failed to stat local file %s: %v", localPath, err)
			continue
		}

		// Fetch full metadata (including custom mod time)
		stat, err := client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
		if err != nil {
			log.Printf("Failed to stat object %s: %v", objectName, err)
			continue
		}

		cloudModTime := getCloudModTime(stat)

		// If file doesn't exist locally or is older than cloud, replace it
		if localInfo == nil || cloudModTime.Sub(localInfo.ModTime().UTC()) > timeTolerance {
			log.Printf("Cloud has newer version of %s. Downloading and replacing...", objectName)
			tempPath := filepath.Join(os.TempDir(), objectName+".download")
			err := client.FGetObject(ctx, bucketName, objectName, tempPath, minio.GetObjectOptions{})
			if err != nil {
				log.Printf("Failed to download %s: %v", objectName, err)
				continue
			}

			// Replace local file
			backupAndReplace(localPath, tempPath)

			// Restore original mod time from cloud
			os.Chtimes(localPath, cloudModTime, cloudModTime)
		}
	}
}

func checkCloudAndSync(ctx context.Context, client *minio.Client, filePath string) {
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		return
	}

	objectName := filepath.Base(filePath)

	// Fetch cloud metadata
	stat, err := client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err == nil {
		cloudModTime := getCloudModTime(stat)
		localFileTime := info.ModTime().UTC()
		diff := cloudModTime.Sub(localFileTime)
		if diff > timeTolerance {
			log.Printf("file compare cloudtime: %v localfiletime: %v", cloudModTime, localFileTime)
			log.Printf("Cloud has newer version of %s. Downloading...", objectName)
			tempPath := filepath.Join(os.TempDir(), objectName+".cloud")
			err := client.FGetObject(ctx, bucketName, objectName, tempPath, minio.GetObjectOptions{})
			if err == nil {
				backupAndReplace(filePath, tempPath)

				// Restore original mod time from cloud
				os.Chtimes(filePath, cloudModTime, cloudModTime)
			}
			return
		} else if diff < -timeTolerance {
			log.Printf("file compare cloudtime: %v localfiletime: %v", cloudModTime, localFileTime)
			log.Printf("Local file %s is newer. Uploading...", objectName)
			backupAndUpload(ctx, client, filePath)
			return
		} else {
			// Times are close enough — no sync needed
			return
		}
	}

	// Otherwise, upload local file
	backupAndUpload(ctx, client, filePath)
}

func backupAndReplace(localPath, newPath string) {
	if fileExists(localPath) {
		backupPath, err := createBackupTimeFolder()
		if err != nil {
			log.Printf("failed to create backup folder: %v", err)
			return
		}

		backupFile := filepath.Join(backupPath, filepath.Base(localPath))
		err = copyFile(localPath, backupFile)
		if err != nil {
			log.Printf("failed to backup file %s to %s", localPath, backupFile)
			return
		}
	}
	err := copyFile(newPath, localPath)
	if err != nil {
		log.Printf("failed to copy cloud file %s to %s", newPath, localPath)
		return
	}

	log.Printf("replaced %s with cloud version", localPath)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func backupAndUpload(ctx context.Context, client *minio.Client, filePath string) {
	backupPath, err := createBackupTimeFolder()
	if err != nil {
		log.Printf("failed to create backup folder: %v", err)
		return
	}

	fileName := filepath.Base(filePath)
	backupFile := filepath.Join(backupPath, fileName)

	err = copyFile(filePath, backupFile)
	if err != nil {
		log.Printf("failed to backup file: %s", err)
	}

	uploadToCloud(ctx, client, filePath, fileName)
}

func createBackupTimeFolder() (string, error) {
	err := ensureFolderExists(backupDir)
	if err != nil {
		return "", err
	}

	timeFolder := time.Now().Format("2006-01-02_15-04-05.000000")
	backupPath := filepath.Join(backupDir, timeFolder)
	err = os.Mkdir(backupPath, os.ModePerm)
	return backupPath, err
}

func uploadToCloud(ctx context.Context, client *minio.Client, filePath, objectName string) {
	exists, err := client.BucketExists(ctx, bucketName)
	if err != nil {
		log.Printf("error checking bucket: %v", err)
		return
	}
	if !exists {
		err = client.MakeBucket(ctx, bucketName, minio.MakeBucketOptions{})
		if err != nil {
			log.Printf("failed to create bucket: %v", err)
			return
		}
	}

	modTime := time.Now().UTC()
	info, err := os.Stat(filePath)
	if err == nil {
		modTime = info.ModTime().UTC()
	}

	// Store full Unix nanoseconds timestamp in metadata
	userMeta := map[string]string{
		"X-Amz-Meta-Modtime":       fmt.Sprintf("%d", modTime.UnixNano()),
		"X-Amz-Meta-ModtimeString": fmt.Sprintf("%v", modTime.Format("2006-01-02_15-04-05.000000")),
	}

	_, err = client.FPutObject(ctx, bucketName, objectName, filePath, minio.PutObjectOptions{
		UserMetadata: userMeta,
	})
	if err != nil {
		log.Printf("Failed to upload file: %v", err)
	} else {
		log.Printf("Uploaded file %s to bucket %s", objectName, bucketName)
	}
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func ensureFolderExists(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Folder does not exist, create it
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			return err
		}
		log.Printf("Created folder: %s", path)
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a folder: %s", path)
	}
	// Folder already exists
	return nil
}

func getCloudModTime(stat minio.ObjectInfo) time.Time {
	cloudModTime := stat.LastModified

	rawModTime := stat.UserMetadata["Modtime"]
	if rawModTime != "" {
		ts, err := strconv.ParseInt(rawModTime, 10, 64)
		if err != nil {
			log.Printf("Invalid modtime metadata %q: %v", rawModTime, err)
		} else {
			// Convert nanoseconds since epoch to time.Time
			cloudModTime = time.Unix(0, ts)
		}
	}

	return cloudModTime.UTC()
}
