//go:build linux

package clipboard

import (
	"errors"
	"fmt"
	"os"
	"wox/util"
)

// kdeWaylandClipboard prefers ext-data-control-v1 on KDE/Plasma Wayland, which
// needs no permission. The portal is only a fallback: it reads the clipboard
// through a RemoteDesktop session, whose consent prompt KDE builds solely from
// requested device types and screen sharing, so a clipboard-only session renders
// an empty body. Images still go through the focused UI GTK path because KWin
// does not reliably request image payloads from a background portal session.
type kdeWaylandClipboard struct{}

// portalFallbackNeeded keeps an empty clipboard, which is a valid answer rather
// than a protocol failure, from escalating to the portal consent prompt.
func portalFallbackNeeded(err error) bool {
	return err != nil && !errors.Is(err, noDataErr)
}

func newKDEWaylandClipboard() kdeWaylandClipboard {
	return kdeWaylandClipboard{}
}

func (kdeWaylandClipboard) name() string {
	return "kde-wayland"
}

func (kdeWaylandClipboard) readContentType() Type {
	if contentType := dataControlReadContentType(); contentType != "" {
		return contentType
	}
	if err := portalReady(); err == nil {
		return portalReadContentType()
	}
	return ""
}

func (kdeWaylandClipboard) readText() (string, error) {
	text, err := dataControlReadText()
	if !portalFallbackNeeded(err) {
		return text, err
	}
	if portalErr := portalReady(); portalErr == nil {
		return portalReadText()
	}
	return text, err
}

func (kdeWaylandClipboard) readFilePaths() ([]string, error) {
	paths, err := dataControlReadFilePaths()
	if !portalFallbackNeeded(err) {
		return paths, err
	}
	if portalErr := portalReady(); portalErr == nil {
		return portalReadFilePaths()
	}
	return paths, err
}

func (kdeWaylandClipboard) readImageSnapshot() (*ImageSnapshot, error) {
	img, err := dataControlReadImageSnapshot()
	if !portalFallbackNeeded(err) {
		return img, err
	}
	if portalErr := portalReady(); portalErr == nil {
		return portalReadImageSnapshot()
	}
	return img, err
}

func (kdeWaylandClipboard) writeText(text string) error {
	err := waylandCopy(portalMimeTextUTF8, []byte(text))
	if err == nil {
		return nil
	}
	if portalErr := portalReady(); portalErr == nil {
		return portalWriteText(text)
	}
	return err
}

func (kdeWaylandClipboard) writeFilePaths(paths []string) error {
	err := newWaylandClipboard().writeFilePaths(paths)
	if err == nil {
		return nil
	}
	if portalErr := portalReady(); portalErr == nil {
		return portalWriteFilePaths(paths)
	}
	return err
}

func (kdeWaylandClipboard) writeImageBytes(pngData []byte) error {
	if err := writeKDEWaylandImageBytesViaUI(pngData); err != nil {
		return err
	}
	util.GetLogger().Info(util.NewTraceContext(), fmt.Sprintf("clipboard: KDE Wayland image write via UI, pngBytes=%d", len(pngData)))
	return nil
}

// isChanged runs on the watcher's timer, so it must not reach portalReady and
// raise the consent prompt with no user action to explain it.
func (kdeWaylandClipboard) isChanged() bool {
	return dataControlIsChanged()
}

func (kdeWaylandClipboard) watchSnapshot() string {
	return dataControlWatchSnapshot()
}

// writeKDEWaylandImageBytesViaUI delegates image clipboard ownership to UI.
func writeKDEWaylandImageBytesViaUI(pngData []byte) error {
	cacheDir := util.GetLocation().GetCacheDirectory()
	if cacheDir == "" {
		cacheDir = os.TempDir()
	}
	if mkdirErr := os.MkdirAll(cacheDir, 0700); mkdirErr != nil {
		return fmt.Errorf("clipboard: failed to create temporary image directory for KDE Wayland UI write: %w", mkdirErr)
	}

	tempFile, err := os.CreateTemp(cacheDir, "wox-clipboard-*.png")
	if err != nil {
		return fmt.Errorf("clipboard: failed to create temporary image file for KDE Wayland UI write: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err = tempFile.Write(pngData); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("clipboard: failed to write temporary image file for KDE Wayland UI write: %w", err)
	}
	if err = tempFile.Close(); err != nil {
		return fmt.Errorf("clipboard: failed to close temporary image file for KDE Wayland UI write: %w", err)
	}

	if err = writeNativeImageFile(util.NewTraceContext(), tempPath); err != nil {
		return fmt.Errorf("clipboard: KDE Wayland UI image write failed: %w", err)
	}
	return nil
}
