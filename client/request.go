package client

import "net/url"

func encodeForm(values map[string]string) string {
	formData := url.Values{}
	for k, v := range values {
		formData.Set(k, v)
	}
	return formData.Encode()
}
