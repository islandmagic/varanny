package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"
)

type DbfsLevel struct {
	Level float64
}

// Compute RMS of a buffer
func computeRMS(in []int16) float64 {
	var sum float64
	sum = 0.0
	for _, v := range in {
		sum += float64(v) * float64(v)
	}
	if sum == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(len(in)))
}

// Compute dBFS level of a buffer
func computedBFS(in []int16) float64 {
	rms := computeRMS(in)
	if rms == 0 {
		// clip to lowest value
		return -96.0
	}
	dbfs := 20 * math.Log10(rms/float64(1<<15))
	return dbfs
}

func systemByteOrder() binary.ByteOrder {
	var i int32 = 0x01020304
	u := unsafe.Pointer(&i)
	pb := (*byte)(u)
	b := *pb

	if b == 0x04 {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func alignTo16BitBuffer(b []byte) ([]int16, error) {
	buf := bytes.NewBuffer(b)
	var data []int16

	for buf.Len() > 0 {
		var value int16
		err := binary.Read(buf, systemByteOrder(), &value)
		if err != nil {
			return nil, err
		}
		data = append(data, value)
	}

	return data, nil
}

/*
Listen to a device and call back a lambda function with the dBFS level
When lambda returns false, the listening stops
*/
func Monitor(deviceInfo malgo.DeviceInfo, dbfsLevels chan DbfsLevel, stop chan bool) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		fmt.Printf(message)
	})
	chk(err)
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()

	// Commonly supported audio format
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = 1
	deviceConfig.PeriodSizeInMilliseconds = 100 // Slow samlping rate
	deviceConfig.SampleRate = 44100
	deviceConfig.Alsa.NoMMap = 1
	deviceConfig.Capture.DeviceID = deviceInfo.ID.Pointer()

	//sizeInBytes := uint32(malgo.SampleSizeInBytes(deviceConfig.Capture.Format))
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {
		// realign the buffer from bytes to int16
		buffer, _ := alignTo16BitBuffer(pSample)
		dbfs := -96.0
		if len(buffer) > 0 {
			dbfs = computedBFS(buffer)
		}
		dbfsLevels <- DbfsLevel{dbfs}
	}

	captureCallbacks := malgo.DeviceCallbacks{
		Data: onRecvFrames,
	}
	device, err := malgo.InitDevice(ctx.Context, deviceConfig, captureCallbacks)
	chk(err)

	err = device.Start()
	chk(err)

	for {
		select {
		case <-stop:
			fmt.Println("Stopping monitoring...")
			device.Uninit()
			return
		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func sanitize(input string) string {
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
	sanitized := reg.ReplaceAllString(input, "")
	return sanitized
}

// Find device based on name
func FindAudioDevice(name string) (malgo.DeviceInfo, error) {
	context, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		fmt.Printf(message)
	})
	chk(err)
	defer func() {
		_ = context.Uninit()
		context.Free()
	}()

	infos, err := context.Devices(malgo.Capture)
	chk(err)

	log.Println("Found capture audio devices: ", len(infos))
	for _, info := range infos {
		log.Println("Device name: ", info.Name())
		// Vara truncate the name to 32 characters
		// match if the name is a prefix of the device name
		if strings.HasPrefix(sanitize(info.Name()), sanitize(name)) {
			return info, nil
		}

		// Linux can add some prefix to the device name in the .ini
		if strings.Contains(sanitize(name), sanitize(info.Name())) {
			return info, nil
		}
	}
	return malgo.DeviceInfo{}, fmt.Errorf("device %s not found", name)
}

func chk(err error) {
	if err != nil {
		panic(err)
	}
}
