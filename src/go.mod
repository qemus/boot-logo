module github.com/qemus/boot-logo

go 1.25

require (
	github.com/google/renameio/v2 v2.0.2
	github.com/linuxboot/fiano v1.2.0
	golang.org/x/image v0.44.0
)

replace github.com/linuxboot/fiano => github.com/qemus/fiano v1.2.0-3

require (
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/ulikunitz/xz v0.5.14 // indirect
	golang.org/x/text v0.40.0 // indirect
)
