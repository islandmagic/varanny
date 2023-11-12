package main

import (
	"fmt"
	"math"
	"os"

	"text/template"

	"github.com/gordonklaus/portaudio"
	"github.com/mjibson/go-dsp/fft"
)

func dump() {
	in := make([]float32, 64)
	stream, err := portaudio.OpenDefaultStream(1, 0, 44100, len(in), in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not open default stream:", err)
		os.Exit(1)
	}

	defer stream.Close()

	err = stream.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not start stream:", err)
		os.Exit(1)
	}

	for {
		err = stream.Read()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not read from stream:", err)
			os.Exit(1)
		}

		// Process audio to display db level

		// Convert in to []float64
		in64 := make([]float64, len(in))
		for i := range in {
			in64[i] = float64(in[i])
		}

		// FFT
		fftData := fft.FFTReal(in64)

		// Calculate power
		power := make([]float64, len(fftData))
		for i := range fftData {
			re := real(fftData[i])
			im := imag(fftData[i])
			power[i] = re*re + im*im
		}

		// Calculate db
		db := make([]float64, len(fftData))
		for i := range fftData {
			db[i] = 10 * math.Log10(power[i])
		}

		// Calculate average
		avg := 0.0
		for i := range db {
			avg += db[i]
		}
		avg /= float64(len(db))

		// Display db level
		fmt.Printf("avg: %f\n", avg)

	}
}

var tmpl = template.Must(template.New("").Parse(
	`{{. | len}} host APIs: {{range .}}
	Name:                   {{.Name}}
	{{if .DefaultInputDevice}}Default input device:   {{.DefaultInputDevice.Name}}{{end}}
	{{if .DefaultOutputDevice}}Default output device:  {{.DefaultOutputDevice.Name}}{{end}}
	Devices: {{range .Devices}}
		Name:                      {{.Name}}
		MaxInputChannels:          {{.MaxInputChannels}}
		MaxOutputChannels:         {{.MaxOutputChannels}}
		DefaultLowInputLatency:    {{.DefaultLowInputLatency}}
		DefaultLowOutputLatency:   {{.DefaultLowOutputLatency}}
		DefaultHighInputLatency:   {{.DefaultHighInputLatency}}
		DefaultHighOutputLatency:  {{.DefaultHighOutputLatency}}
		DefaultSampleRate:         {{.DefaultSampleRate}}
	{{end}}
{{end}}`,
))

func main() {
	portaudio.Initialize()
	defer portaudio.Terminate()
	hs, err := portaudio.HostApis()
	chk(err)
	err = tmpl.Execute(os.Stdout, hs)
	chk(err)
	dump()
}

func chk(err error) {
	if err != nil {
		panic(err)
	}
}
