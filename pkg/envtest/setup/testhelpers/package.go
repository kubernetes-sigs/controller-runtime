package testhelpers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5" //nolint:gosec
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"path/filepath"
)

func ContentsFor(filename string) ([]byte, error) { //nolint:revive
	var chunk [1024 * 48]byte // 1.5 times our chunk read size in GetVersion
	copy(chunk[:], filename)
	if _, err := rand.Read(chunk[len(filename):]); err != nil {
		return nil, err
	}

	out := new(bytes.Buffer)
	gzipWriter := gzip.NewWriter(out)

	tarWriter := tar.NewWriter(gzipWriter)

	if err := tarWriter.WriteHeader(&tar.Header{
		Name: filepath.Join("kubebuilder/bin", filename),
		Size: int64(len(chunk)),
		Mode: 0777, // so we can check that we fix this later
	}); err != nil {
		return nil, fmt.Errorf("write tar header: %w", err)
	}
	if _, err := tarWriter.Write(chunk[:]); err != nil {
		return nil, fmt.Errorf("write tar contents: %w", err)
	}

	// can't defer these, because they need to happen before out.Bytes() below
	tarWriter.Close()
	gzipWriter.Close()

	return out.Bytes(), nil
}

func verWith(name string, contents []byte) Item {
	res := Item{
		Meta:     BucketObject{Name: name},
		Contents: contents,
	}
	hash := md5.Sum(res.Contents) //nolint:gosec
	res.Meta.Hash = base64.StdEncoding.EncodeToString(hash[:])
	return res
}
