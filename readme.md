<h1 align="center">boot-logo<br />
<div align="center">
<a href="https://github.com/qemus/boot-logo"><img src="https://raw.githubusercontent.com/qemus/boot-logo/master/.github/logo.png" title="Logo" style="max-width:100%;" width="96" /></a>
</div>
<div align="center">

[![Build]][build_url]
[![Version]][release_url]
[![Size]][release_url]

</div></h1>

A small command-line tool for extracting and replacing the boot logo embedded in OVMF firmware.

## Features ✨

- Replaces embedded bitmaps with a custom image
- Preserves all unchanged firmware sections
- Extracts bitmaps from TianoCore firmware files
- Available for both AMD64 and ARM64 platforms

## Usage

### Replace the boot logo

```bash
boot-logo logo.bmp firmware.fd
```

The `replace` command may also be specified explicitly:

```bash
boot-logo replace logo.bmp firmware.fd
```

By default, the modified firmware is written beside the original file:

```text
firmware.fd
firmware.boot-logo.fd
```

Set a custom output path:

```bash
boot-logo replace logo.bmp firmware.fd --output modified.fd
```

Overwrite the input firmware:

```bash
boot-logo replace logo.bmp firmware.fd --in-place
```

### Extract the current boot logo

```bash
boot-logo extract firmware.fd
```

By default, the extracted bitmap is written to:

```text
firmware.fd.logo.bmp
```

Set a custom output path:

```bash
boot-logo extract firmware.fd --output logo.bmp
```

### Other options

```text
-h, --help       Show usage information
-v, --version    Show version information
```

## Image requirements

The replacement image must be an uncompressed BMP file.

Supported color depths:

```text
1, 4, 8, 16, 24 and 32 bits per pixel
```

The firmware must contain exactly one valid bitmap inside the TianoCore `LogoDxe` file:

```text
F74D20EE-37E7-48FC-97F7-9B1047749C69
```

The bitmap must be stored in a raw firmware section.

## Installation

Download the binary for your architecture from the [latest release][release_url]:

```text
boot-logo-amd64.bin
boot-logo-arm64.bin
```

Make it executable:

```bash
chmod +x boot-logo-amd64.bin
```

Optionally install it system-wide:

```bash
sudo install -m 755 boot-logo-amd64.bin /usr/local/bin/boot-logo
```

## Warning

Firmware modification always carries risk. Keep an untouched copy of the original firmware and verify the generated image before using it.

This tool currently targets firmware containing the standard TianoCore `LogoDxe` layout. Vendor-specific firmware may store its logo differently and will be rejected instead of being modified blindly.

## Stars 🌟

[![Stargazers](https://raw.githubusercontent.com/star-stats/stars/refs/heads/data/charts/qemus-boot-logo.svg)](https://github.com/qemus/boot-logo/stargazers)

[Fiano]: https://github.com/linuxboot/fiano
[build_url]: https://github.com/qemus/boot-logo/
[release_url]: https://github.com/qemus/boot-logo/releases

[Build]: https://github.com/qemus/boot-logo/actions/workflows/build.yml/badge.svg
[Size]: https://img.shields.io/badge/size-4.14_MB-steelblue?style=flat&color=066da5
[Version]: https://img.shields.io/github/v/tag/qemus/boot-logo?label=version&sort=semver&color=066da5
