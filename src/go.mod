module github.com/qemus/boot-logo

go 1.25

require (
	github.com/google/renameio/v2 v2.0.2
	github.com/linuxboot/fiano v1.2.0
)

replace github.com/linuxboot/fiano => github.com/qemus/fiano v0.0.0-20260801193248-482fe58a488a

require (
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/ulikunitz/xz v0.5.14 // indirect
	golang.org/x/text v0.6.0 // indirect
)
