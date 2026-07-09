/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package remote

import (
	"crypto/md5"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"

	"sigs.k8s.io/controller-runtime/tools/setup-envtest/versions"
)

func readBody(resp *http.Response, out io.Writer, archiveName string, platform versions.PlatformItem) error {
	if platform.Hash != nil {
		// stream in chunks to do the checksum, don't load the whole thing into
		// memory to avoid causing issues with big files.
		buf := make([]byte, 32*1024) // 32KiB, same as io.Copy
		var hasher hash.Hash
		switch platform.Hash.Type {
		case versions.SHA512HashType:
			hasher = sha512.New()
		case versions.MD5HashType:
			hasher = md5.New()
		default:
			return fmt.Errorf("hash type %s not implemented", platform.Hash.Type)
		}
		for cont := true; cont; {
			amt, err := resp.Body.Read(buf)
			if err != nil && !errors.Is(err, io.EOF) {
				return fmt.Errorf("unable read next chunk of %s: %w", archiveName, err)
			}
			if amt > 0 {
				// checksum never returns errors according to docs
				hasher.Write(buf[:amt])
				if _, err := out.Write(buf[:amt]); err != nil {
					return fmt.Errorf("unable write next chunk of %s: %w", archiveName, err)
				}
			}
			cont = amt > 0 && !errors.Is(err, io.EOF)
		}

		var sum string
		switch platform.Hash.Encoding {
		case versions.Base64HashEncoding:
			sum = base64.StdEncoding.EncodeToString(hasher.Sum(nil))
		case versions.HexHashEncoding:
			sum = hex.EncodeToString(hasher.Sum(nil))
		default:
			return fmt.Errorf("hash encoding %s not implemented", platform.Hash.Encoding)
		}
		if sum != platform.Hash.Value {
			return fmt.Errorf("checksum mismatch for %s: %s (computed) != %s (reported)", archiveName, sum, platform.Hash.Value)
		}
	} else if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("unable to download %s: %w", archiveName, err)
	}
	return nil
}
