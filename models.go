package main

type HostInfo struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type ManifestPage struct {
	ID          string `json:"id"`
	VolumeID    string `json:"volumeId"`
	Index       int    `json:"index"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	FileSize    int64  `json:"fileSize"`
}

type ManifestVolume struct {
	ID           string         `json:"id"`
	SeriesID     string         `json:"seriesId"`
	Title        string         `json:"title"`
	VolumeNumber *int           `json:"volumeNumber"`
	PageCount    int            `json:"pageCount"`
	Pages        []ManifestPage `json:"pages"`
}

type ManifestSeries struct {
	ID      string           `json:"id"`
	Title   string           `json:"title"`
	Volumes []ManifestVolume `json:"volumes"`
}

type RegisterManifestMessage struct {
	Type   string           `json:"type"`
	Host   HostInfo         `json:"host"`
	Series []ManifestSeries `json:"series"`
}

type PageResponseHeader struct {
	Type        string `json:"type"`
	RequestID   string `json:"requestId"`
	PageID      string `json:"pageId"`
	ContentType string `json:"contentType"`
}

type PageErrorMessage struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	PageID    string `json:"pageId"`
	Error     string `json:"error"`
}
