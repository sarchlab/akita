package timing

// FreqInHz defines frequency in the unit of Hertz (cycles per second).
type FreqInHz uint64

const (
	Hz = FreqInHz(1)
	kHz = FreqInHz(1000 * Hz)
	MHz = FreqInHz(1000 * kHz)
	GHz = FreqInHz(1000 * MHz
)

var globalFreqDomains = GHz

func RegisterFreqDomain(freq FreqInHz) {

}

func CycleToTime(cycle uint64) VTimeInSec {
}
