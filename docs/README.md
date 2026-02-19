# Panago Documentation

See the [root README](../README.md) for installation and usage.

## Components

- [Firmware package](../pkg/firmware/) — decode/encode Panasonic DP-UB9000 firmware
- [Cramfs package](../pkg/cramfs/) — cramfs filesystem read/write
- [Romfs package](../pkg/romfs/) — romfs filesystem read/write
- [CLI commands](../cmd/) — cobra command definitions

## Firmware Internals

A full description of the firmware encryption layers, partition structure, MAIN
sub-entry layout, LZSS compression parameters, and all checksum algorithms is in
the **Technical Details** section of the [root README](../README.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).
