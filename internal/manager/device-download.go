package manager

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/lindell/connect-iq-sdk-manager-cli/internal/client"
	"github.com/lindell/connect-iq-sdk-manager-cli/internal/connectiq"
	log "github.com/sirupsen/logrus"
)

type DownloadConfig struct {
	DeviceFilters DeviceFilters
	IncludeFonts  bool
}

func (m *Manager) Download(ctx context.Context, config DownloadConfig) error {
	deviceInfos, err := getDeviceInfo(ctx)
	if err != nil {
		return err
	}

	deviceInfos, err = filterDevices(deviceInfos, config.DeviceFilters)
	if err != nil {
		return err
	}

	log.Infof("Downloading %d devices", len(deviceInfos))

	var (
		mu           sync.Mutex
		hashEntities []connectiq.ConfigEntity
	)

	err = runConcurrent(ctx, deviceInfos, func(ctx context.Context, device client.DeviceInfo) error {
		logger := log.WithField("device", device.Name)
		hash, err := m.fetchDevice(ctx, logger, device)
		if err != nil {
			return err
		}
		if hash != "" {
			mu.Lock()
			hashEntities = append(hashEntities, connectiq.ConfigEntity{
				Key:   fmt.Sprintf("%s-hash", device.Name),
				Value: hash,
			})
			mu.Unlock()
		}
		return nil
	})
	if len(hashEntities) > 0 {
		if storeErr := connectiq.StoreConfigKeyVals(hashEntities...); storeErr != nil {
			log.Errorf("Could not store device hashes: %s", storeErr)
		}
	}
	if err != nil {
		return err
	}

	if config.IncludeFonts {
		if err := m.downloadFonts(ctx, deviceInfos); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) fetchDevice(ctx context.Context, log log.FieldLogger, device client.DeviceInfo) (string, error) {
	deviceDir := path.Join(connectiq.DevicesPath, device.Name)

	if _, err := os.Stat(deviceDir); !os.IsNotExist(err) {
		log.Info("Device folder already exist")
		return "", nil
	}

	log.Info("Downloading device zip")
	r, err := client.DownloadDevice(ctx, device)
	if err != nil {
		return "", err
	}
	defer r.Close()

	return fetchAndExtract(r, deviceDir, "device.zip")
}
