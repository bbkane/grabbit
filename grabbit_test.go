package main

import "testing"

func Test_validateImageURL(t *testing.T) {
	type args struct {
		fullURL string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name:    "bare",
			args:    args{fullURL: "https://example.com/img.jpeg"},
			want:    "img.jpeg",
			wantErr: false,
		},
		{
			name:    "query",
			args:    args{fullURL: "https://example.com/img.jpeg?abc=def"},
			want:    "img.jpeg",
			wantErr: false,
		},
		{
			name:    "bad",
			args:    args{fullURL: "https://example.com/hellodarknessmyoldfriend"},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateImageURL(tt.args.fullURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateImageURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("validateImageURL() = %v, want %v", got, tt.want)
			}
		})
	}
}
