<h1 align="center">boot-logo<br />
<div align="center">
<a href="https://github.com/qemus/boot-logo"><img src="https://raw.githubusercontent.com/qemus/boot-logo/master/.github/logo.png" title="Logo" style="max-width:100%;" width="96" /></a>
</div>
<div align="center">

[![Build]][build_url]
[![Version]][release_url]
[![Size]][release_url]

</div></h1>

A small command-line tool for extracting and replacing the boot logo embedded in OVMF firmware and FFS files.

## Features ✨

- Replaces embedded logos with a custom image
- Supports BMP, PNG, JPG and JPEG images
- Preserves all unchanged firmware sections
- Extracts bitmaps from firmware and FFS files
- Available for both AMD64 and ARM64 platforms

## Usage

### Replace the boot logo

```bash
boot-logo logo.png firmware.fd
```

The `replace` command can also be specified explicitly:

```bash
boot-logo replace logo.png firmware.fd
```

The input image may be a BMP, PNG, JPG or JPEG file.

By default, the supplied firmware file is modified in place.

To write the modified firmware to a different file instead:

```bash
boot-logo replace logo.jpg firmware.fd --output modified.fd
```

### Extract the boot logo

```bash
boot-logo extract firmware.fd
```

By default, the extracted logo is written to:

```text
firmware.fd.logo.bmp
```

Specify a different output path:

```bash
boot-logo extract firmware.fd --output logo.bmp
```

A logo can also be extracted from a standalone FFS file:

```bash
boot-logo extract LogoDxe.ffs
```

### Options

```text
-o, --output <path>  Write to a different output path
-h, --help           Show usage information
-v, --version        Show version information
```

## Firmware support

The tool supports complete OVMF firmware images and standalone FFS files containing the standard TianoCore `LogoDxe` file:

```text
F74D20EE-37E7-48FC-97F7-9B1047749C69
```

The tool expects exactly one valid bitmap inside this file. Firmware that does not match this layout is rejected instead of being modified blindly.

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

## Stars 🌟

[![Stargazers](https://raw.githubusercontent.com/star-stats/stars/refs/heads/data/charts/qemus-boot-logo.svg)](https://github.com/qemus/boot-logo/stargazers)

[Fiano]: https://github.com/linuxboot/fiano
[build_url]: https://github.com/qemus/boot-logo/
[release_url]: https://github.com/qemus/boot-logo/releases

[Build]: https://github.com/qemus/boot-logo/actions/workflows/build.yml/badge.svg
[Size]: https://img.shields.io/badge/size-4.14_MB-steelblue?style=flat&color=066da5
[Version]: https://img.shields.io/github/v/tag/qemus/boot-logo?label=version&sort=semver&color=066da5
