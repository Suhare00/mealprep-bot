package seed

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

type csvRow struct {
	number int
	fields []string
}

func readCSV(directory, filename string, expectedHeader []string) ([]csvRow, error) {
	path := filepath.Join(directory, filename)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s: open: %w", filename, err)
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: row 1: header is required", filename)
	}
	if err != nil {
		return nil, csvReadError(filename, 1, err)
	}
	if !reflect.DeepEqual(header, expectedHeader) {
		return nil, fmt.Errorf("%s: row 1: unexpected header: got %q, want %q", filename, header, expectedHeader)
	}
	reader.FieldsPerRecord = len(expectedHeader)

	rows := make([]csvRow, 0)
	for rowNumber := 2; ; rowNumber++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, csvReadError(filename, rowNumber, err)
		}
		rows = append(rows, csvRow{number: rowNumber, fields: record})
	}

	return rows, nil
}

func csvReadError(filename string, fallbackRow int, err error) error {
	var parseError *csv.ParseError
	if errors.As(err, &parseError) && parseError.Line > 0 {
		return fmt.Errorf("%s: row %d: %w", filename, parseError.Line, err)
	}
	return fmt.Errorf("%s: row %d: %w", filename, fallbackRow, err)
}
