package app

import "mime"

func registerWebAssetMIMETypes() error {
	for extension, contentType := range map[string]string{
		".mjs":  "text/javascript; charset=utf-8",
		".wasm": "application/wasm",
	} {
		if err := mime.AddExtensionType(extension, contentType); err != nil {
			return err
		}
	}

	return nil
}
