package datarecording_test

import (
	"os"
	"testing"

	"github.com/sarchlab/akita/v4/datarecording"
	"github.com/stretchr/testify/assert"
)

// Sample struct with Location field
type Sample struct {
	Name     string
	ID       int
	Location string
}

// NewSample represents Sample struct with Location converted to int indexing
type NewSample struct {
	Name     string
	ID       int
	Location int
}

// Struct mapping to location table
type location struct {
	index int
	where string
}

func TestDataRecorderWithLocation(t *testing.T) {
	var expectedSamples []Sample
	var locs []string

	dbPath := "test_location"
	os.Remove(dbPath + ".sqlite3")

	recorder := datarecording.NewDataRecorder(dbPath)

	sampleEntry := Sample{"One", 1, "A"}
	recorder.CreateTable("test_table", sampleEntry)
	recorder.InsertData("test_table", sampleEntry)
	expectedSamples = append(expectedSamples, sampleEntry)

	entryTwo := Sample{"Two", 2, "B"}
	recorder.InsertData("test_table", entryTwo)
	expectedSamples = append(expectedSamples, entryTwo)

	entryThree := Sample{"Three", 3, "A"}
	recorder.InsertData("test_table", entryThree)
	expectedSamples = append(expectedSamples, entryThree)

	entryFour := Sample{"Four", 4, "C"}
	recorder.InsertData("test_table", entryFour)
	expectedSamples = append(expectedSamples, entryFour)
	recorder.Flush()

	recorder.Close()

	reader := datarecording.NewReader(dbPath)

	tables := reader.ListTables()
	assert.Equal(t, tables[0], "test_table")
	assert.Equal(t, tables[1], "test_table_location")

	reader.MapTable("test_table_location", location{})
	locResults, count, err := reader.Query("test_table_location", datarecording.QueryParams{})
	if err != nil {
		panic(err)
	}

	assert.Equal(t, 3, count)

	for _, locResult := range locResults {
		loc := locResult.(*location)
		locs = append(locs, loc.where)
	}

	reader.MapTable("test_table", NewSample{})
	results, _, err := reader.Query("test_table", datarecording.QueryParams{})
	if err != nil {
		panic(err)
	}

	for record, result := range results {
		sample := result.(*NewSample)
		locIndex := sample.Location
		expectedSample := expectedSamples[record]
		assert.True(t, expectedSample.Location == locs[locIndex])
	}

	reader.Close()

}
