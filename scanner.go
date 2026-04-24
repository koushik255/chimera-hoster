package main

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	imageExtensions      = map[string]struct{}{".jpg": {}, ".jpeg": {}, ".png": {}, ".webp": {}}
	ignoredDirectoryName = map[string]struct{}{"_cbz_backups": {}, "__pycache__": {}}
	volumeNumberPattern  = regexp.MustCompile(`(\d+)`)
	slugPattern          = regexp.MustCompile(`[^a-z0-9]+`)
)

type HostedPage struct {
	Path        string
	ContentType string
}

type ManifestSummary struct {
	Series  int
	Volumes int
	Pages   int
}

func buildManifest(config HostConfig) (RegisterManifestMessage, map[string]HostedPage, ManifestSummary, error) {
	seriesList := make([]ManifestSeries, 0, len(config.SeriesPaths))
	pageLookup := make(map[string]HostedPage)
	totalVolumes := 0

	for _, seriesPath := range config.SeriesPaths {
		series, pagesForSeries, err := scanSeries(seriesPath)
		if err != nil {
			return RegisterManifestMessage{}, nil, ManifestSummary{}, err
		}
		for pageID, page := range pagesForSeries {
			if _, exists := pageLookup[pageID]; exists {
				return RegisterManifestMessage{}, nil, ManifestSummary{}, fmt.Errorf("duplicate page id: %s", pageID)
			}
			pageLookup[pageID] = page
		}
		totalVolumes += len(series.Volumes)
		seriesList = append(seriesList, series)
	}

	return RegisterManifestMessage{
			Type:   "register_manifest",
			Host:   config.Host,
			Series: seriesList,
		}, pageLookup, ManifestSummary{
			Series:  len(seriesList),
			Volumes: totalVolumes,
			Pages:   len(pageLookup),
		}, nil
}

func scanSeries(rootPath string) (ManifestSeries, map[string]HostedPage, error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ManifestSeries{}, nil, fmt.Errorf("series path does not exist: %s", rootPath)
		}
		return ManifestSeries{}, nil, fmt.Errorf("stat series path %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		return ManifestSeries{}, nil, fmt.Errorf("series path is not a directory: %s", rootPath)
	}

	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return ManifestSeries{}, nil, fmt.Errorf("read series path %s: %w", rootPath, err)
	}

	seriesID := slugify(filepath.Base(rootPath))
	series := ManifestSeries{
		ID:      seriesID,
		Title:   filepath.Base(rootPath),
		Volumes: make([]ManifestVolume, 0),
	}
	pageLookup := make(map[string]HostedPage)

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ignored := ignoredDirectoryName[entry.Name()]; ignored {
			continue
		}

		volumePath := filepath.Join(rootPath, entry.Name())
		volume, pages, err := scanVolume(seriesID, volumePath, entry.Name())
		if err != nil {
			return ManifestSeries{}, nil, err
		}
		if len(volume.Pages) == 0 {
			continue
		}

		series.Volumes = append(series.Volumes, volume)
		for pageID, page := range pages {
			pageLookup[pageID] = page
		}
	}

	return series, pageLookup, nil
}

func scanVolume(seriesID, volumePath, volumeTitle string) (ManifestVolume, map[string]HostedPage, error) {
	volumeNumber := parseVolumeNumber(volumeTitle)
	volumeID := buildVolumeID(seriesID, volumeNumber, volumeTitle)

	imagePaths := make([]string, 0)
	if err := filepath.WalkDir(volumePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != volumePath {
				if _, ignored := ignoredDirectoryName[d.Name()]; ignored {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if _, ok := imageExtensions[strings.ToLower(filepath.Ext(d.Name()))]; ok {
			imagePaths = append(imagePaths, path)
		}
		return nil
	}); err != nil {
		return ManifestVolume{}, nil, fmt.Errorf("scan volume %s: %w", volumePath, err)
	}

	sort.Slice(imagePaths, func(i, j int) bool {
		left, _ := filepath.Rel(volumePath, imagePaths[i])
		right, _ := filepath.Rel(volumePath, imagePaths[j])
		return left < right
	})

	volume := ManifestVolume{
		ID:           volumeID,
		SeriesID:     seriesID,
		Title:        volumeTitle,
		VolumeNumber: volumeNumber,
		PageCount:    len(imagePaths),
		Pages:        make([]ManifestPage, 0, len(imagePaths)),
	}
	pageLookup := make(map[string]HostedPage, len(imagePaths))

	for index, imagePath := range imagePaths {
		fileInfo, err := os.Stat(imagePath)
		if err != nil {
			return ManifestVolume{}, nil, fmt.Errorf("stat image file %s: %w", imagePath, err)
		}

		pageID := fmt.Sprintf("%s-p%03d", volumeID, index+1)
		contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imagePath)))
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		volume.Pages = append(volume.Pages, ManifestPage{
			ID:          pageID,
			VolumeID:    volumeID,
			Index:       index + 1,
			FileName:    filepath.Base(imagePath),
			ContentType: contentType,
			FileSize:    fileInfo.Size(),
		})
		pageLookup[pageID] = HostedPage{
			Path:        imagePath,
			ContentType: contentType,
		}
	}

	return volume, pageLookup, nil
}

func slugify(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func parseVolumeNumber(name string) *int {
	match := volumeNumberPattern.FindStringSubmatch(name)
	if len(match) < 2 {
		return nil
	}

	var volumeNumber int
	if _, err := fmt.Sscanf(match[1], "%d", &volumeNumber); err != nil {
		return nil
	}
	return &volumeNumber
}

func buildVolumeID(seriesID string, volumeNumber *int, directoryName string) string {
	if volumeNumber != nil {
		return fmt.Sprintf("%s-v%02d", seriesID, *volumeNumber)
	}
	return fmt.Sprintf("%s-%s", seriesID, slugify(directoryName))
}
