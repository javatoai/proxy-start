// Package ui contains the MyGO native interface for Proxy Start.

package ui

import (
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	nativeui "github.com/egoist/mygo/ui"
	"proxy-start/internal/application"
	"proxy-start/internal/domain"
)

const (
	pageLaunch         = 0
	pageProxySettings  = 1
	pageSystemSettings = 2

	operationTimeout = 30 * time.Second
	refreshInterval  = time.Second
)

type model struct {
	desktop  *application.Desktop
	win      *mygo.Window
	tray     *mygo.Tray
	options  Options
	autoQuit *time.Timer

	refresh  chan struct{}
	page     int
	quitting atomic.Bool

	snapshot      application.Snapshot
	status        string
	statusKind    noticeKind
	statusUntil   time.Time
	statusVersion uint64
	toastVersion  uint64
	toastID       string
	busy          map[string]bool
	appSearch     string
	appFilter     string
	selectedAppID string
	icons         map[string]*nativeui.Bitmap
	iconLoading   map[string]bool
	iconAttempts  map[string]string

	drafts  map[string]*appDraft
	details map[string]bool

	installed installedPickerState

	profileID    string
	profileDraft profileDraft
	profileNew   bool
	probes       map[string]domain.ProbeResult

	proxyModalOpen       bool
	deleteModalOpen      bool
	deleteProfileID      string
	deleteReturnToEditor bool
	deleteSessionToken   uint64
	profileEditToken     uint64
	toastDetail          string
	toastDetailOpen      bool
}

type appDraft struct {
	name    string
	proxyID string
	related string
}

type profileDraft struct {
	id       string
	name     string
	protocol string
	host     string
	port     string
	username string
	password string
}

func newModel(desktop *application.Desktop) *model {
	return &model{
		desktop:      desktop,
		busy:         make(map[string]bool),
		drafts:       make(map[string]*appDraft),
		details:      make(map[string]bool),
		probes:       make(map[string]domain.ProbeResult),
		appFilter:    "all",
		icons:        make(map[string]*nativeui.Bitmap),
		iconLoading:  make(map[string]bool),
		iconAttempts: make(map[string]string),
		refresh:      make(chan struct{}),
		installed: installedPickerState{
			icons:     make(map[string]*nativeui.Bitmap),
			attempts:  make(map[string]bool),
			iconSlots: make(chan struct{}, 4),
		},
	}
}
