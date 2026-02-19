# panago

Golang toolkit for Panasonic DP-UB9000 (and similar) UHD Blu-ray player research and modification.

## Features

- **Firmware Tools** - Decode, encode, and analyze Panasonic firmware files (PANAEUSB.FRM)
- **Cramfs Tools** - Extract and create cramfs filesystem images
- **Romfs Tools** - Extract and create romfs filesystem images
- **Device Discovery** - Find Panasonic players on the network via SSDP
- **USB Disk Management** - List, format, and write disk images to USB drives
- **TUI Application** - Graphical terminal interface for device management

## Installation

### From Source

```bash
git clone https://github.com/tridentsx/panago.git
cd panago
make build
```

### Pre-built Binaries

Download from the [Releases](https://github.com/tridentsx/panago/releases) page.

## Binaries

The project builds two binaries:

- **`panago`** - TUI (terminal UI) for interactive device management
- **`panago-cli`** - Command-line interface with all tools as subcommands

## Usage

### Unified CLI (`panago-cli`)

```bash
panago-cli [command] [subcommand] [options]
```

---

### Simple Workflow (Recommended)

Extract everything, modify files, rebuild in three steps:

```bash
# 1. Extract firmware and all filesystems
panago-cli extract PANAEUSB.FRM ./workspace/

# 2. Modify files in the workspace
#    ./workspace/fma5/  = root filesystem (cramfs)
#    ./workspace/fma6/  = data partition  (romfs)
#    ./workspace/fma7/  = app filesystem  (cramfs)

# 3. Rebuild and repack firmware
panago-cli build ./workspace/ PANAEUSB_modified.FRM PANAEUSB.FRM
```

Workspace structure after extraction:

```
workspace/
├── PROG_0.00.bin          (boot program)
├── MINI_7.74.bin          (mini partition)
├── DRV1_D110.bin          (driver partition 1)
├── DRV1_V304.bin          (driver partition 2)
├── BUCD_000.bin           (BD certification data)
├── MAIN_metadata.json     (MAIN encoding parameters - do not edit)
├── fma4.bin               (kernel - kept as binary)
├── fma5/                  (root filesystem - editable)
│   ├── sbin/init
│   ├── etc/
│   └── ...
├── fma6/                  (data partition - editable)
│   ├── local/fonts/
│   └── ...
└── fma7/                  (app filesystem - editable)
    ├── bin/
    └── ...
```

---

### Firmware Tools

#### Show Firmware Information

```bash
panago-cli firmware info PANAEUSB.FRM
```

```
Firmware: PANAEUSB.FRM
Size: 184193072 bytes
Partitions: 6

Name     Version        Offset         Size
-------- -------- ------------ ------------
PROG     0.00             8192       327680
MAIN     3820           335872    180688644
MINI     7.74        181026816       524288
DRV1     D110        181551104      1122688
DRV1     V304        182677504      1122688
BUCD     000         183803904       389120
```

#### Extract Firmware Partitions

```bash
panago-cli firmware decode PANAEUSB.FRM ./extracted/
```

Extracts all partitions:
- `PROG_0.00.bin` - Boot program
- `MAIN.bin` - Main data (concatenated kernel + filesystems)
- `MAIN_metadata.json` - MAIN encoding parameters (required for re-encoding)
- `MINI_7.74.bin` - Mini partition
- `DRV1_D110.bin`, `DRV1_V304.bin` - Driver partitions
- `BUCD_000.bin` - BD certification data

#### Rebuild Firmware

```bash
panago-cli firmware encode ./modified/ output.FRM original.FRM
```

Uses the original firmware as a structural template. Non-MAIN partitions must match
the naming convention `NAME_VERSION.bin` and the exact original size. The MAIN
partition is re-encoded using `MAIN.bin` + `MAIN_metadata.json`.

#### Split MAIN.bin

```bash
panago-cli firmware split-main MAIN.bin ./main_parts/
```

```
Splitting MAIN.bin...
  fma4: offset=0x0, size=6553600 (raw)
  fma5: offset=0x640000, size=47710208 (cramfs)
  fma6: offset=0x33c0000, size=18874368 (romfs)
  fma7: offset=0x45c0000, size=111411200 (cramfs)
```

Sub-images:
- `fma4.bin` - Kernel (raw binary)
- `fma5.bin` - Root filesystem (cramfs)
- `fma6.bin` - Data partition (romfs)
- `fma7.bin` - Application filesystem (cramfs)

#### Combine MAIN.bin

```bash
panago-cli firmware combine-main ./main_parts/ MAIN_modified.bin
```

Concatenates `fma4.bin` through `fma7.bin` in order.

#### Test Crypto Implementations

```bash
panago-cli firmware test
```

Verifies Feistel cipher, AES-128-CBC, and LZSS compression round-trips.

---

### Cramfs Tools

#### List Files

```bash
panago-cli cramfs list fma5.bin
```

```
d0755     0 usr
d0755     0 tmp
l0777     1 armv4t -> .
d0755   296 sbin
-0755 48060 sbin/init
l0777     3 var -> tmp
...
```

#### Extract

```bash
panago-cli cramfs extract fma5.bin ./rootfs/
```

#### Create

```bash
panago-cli cramfs create ./rootfs/ new_fma5.bin
```

#### Info

```bash
panago-cli cramfs info fma5.bin
```

```
Cramfs image: fma5.bin
Files: 171
Directories: 78
Symlinks: 156
Total uncompressed size: 38651830 bytes
```

---

### Romfs Tools

#### List Files

```bash
panago-cli romfs list fma6.bin
```

#### Extract

```bash
panago-cli romfs extract fma6.bin ./data/
```

#### Create

```bash
panago-cli romfs create ./data/ new_fma6.bin
```

#### Info

```bash
panago-cli romfs info fma6.bin
```

```
Romfs image: fma6.bin
Volume name: rom 665d5f54
Image size: 18641888 bytes
Files: 666
Directories: 36
Symlinks: 35
Total content size: 18601986 bytes
```

---

### Device Discovery

```bash
# Discover all UPnP devices on the network
panago-cli discover

# Show only Panasonic players
panago-cli discover --panasonic
```

---

### USB Disk Management

```bash
# List available USB disks
panago-cli disk list

# Write a disk image to a USB device
panago-cli disk write /dev/sdX drive.img.gz
```

---

### TUI Application

```bash
./bin/panago
```

The TUI scans for Panasonic players on startup via SSDP, with manual IP entry as
fallback. Features include device shell access, FPC key extraction, backup, USB disk
creation, and firmware updates.

---

## Technical Details

### Firmware Encryption

Panasonic firmware uses a two-layer encryption scheme applied in sequence:

1. **AES-128-CBC** — outer layer, applied to the entire file
2. **Custom Feistel cipher** — 16-round, 8-byte block cipher with a custom S-box
   - Applied to the 48-byte file header and the 8 KB module header block
   - Applied to the first and last 5 KB (or 10 KB for large firmwares) of each MAIN sub-entry

### Firmware Structure

```
File (AES-CBC encrypted)
└── 0x00: File header (48 bytes, Feistel encrypted)
          [0:4]   = 0x30 (self-size)
          [4:8]   = payload size (file_size - 48)
          [32:44] = "PANASONIC\0\0\0" (product identifier)
          [44:48] = Unix timestamp (firmware build date)
└── 0x30: Module header block (8 KB, Feistel encrypted)
          Entry 0: "$PaT" marker
          Entry 1..N: partition descriptors (48 bytes each)
            [0:4]   Name      (e.g. "MAIN", "PROG")
            [4:8]   Version   (e.g. "3820", "0.00")
            [12:16] Offset    (partition start in file)
            [24:26] TypeFlags (low byte = 0x4D; high: 0=standard, 1=system)
            [32:36] Size      (partition size in bytes)
            [36:40] DataCk    (Adler32 of Feistel-decrypted partition data)
            [40:44] RingBufSize (0 for most; 0x02000000 for MAIN)
            [44:48] EntryCk   (Adler32 of entry bytes [0:44])
└── Partitions: PROG, MAIN, MINI, DRV1, DRV1, BUCD
```

### MAIN Partition Structure

The MAIN partition contains a sub-entry list with individually compressed chunks:

```
MAIN partition
└── 0x00: First header (48 bytes, Feistel encrypted)
└── 0x30: List header (20 bytes, plaintext)
          [0:4]   Checksum      (sum of LE uint32 words from bytes [4:])
          [4:8]   FormatVersion (always 1)
          [8:12]  ListSize      (total list header + entry records size)
          [12:16] DecompSize    (total decompressed size)
          [16:20] CompType      (2 = LZSS, 0 = uncompressed)
└── Entry records (8 bytes each):
          [0:4]   Size      (entry blob size)
          [4:8]   Checksum  (Adler32 of Feistel-encrypted entry blob)
└── Entry blobs (Feistel encrypted at boundaries):
    Each entry:
          [0:14]  Signature     (e.g. "EXTRHEADDRVD  ")
          [14:16] CompType      (2 = raw LZSS)
          [16:20] DecompSize    (uncompressed chunk size)
          [20:24] DestAddr      (load address, 0)
          [24:28] CompSize      (compressed data size)
          [28:32] Slack         (BufferConstant - FooterOffset)
          [32:36] FooterOffset  (64 + align4(CompSize))
          [36:40] BaseAddr      (0)
          [40:44] HdrChecksum   (Adler32 of every 16th byte of decompressed data)
          [44]    ChecksumFlag  (0x10)
          [64:]   LZSS compressed data
          [FooterOffset:] "EXTRFOOT" (8 bytes)
```

### LZSS Compression

The MAIN sub-entries use a variant of the classic Haruhiko Okumura LZSS algorithm:

- 4096-byte ring buffer, initialized to `0x00`
- Initial write position: `0xFEE` (4078)
- Offsets: 12 bits; lengths: 4 bits with bias +3 (range 3–18 bytes)
- Flag byte precedes each group of 8 tokens: `1` = literal, `0` = back-reference
- Back-references to the pre-filled zero window (offsets `0x000`–`0xFED`) are valid
  for the first 4096 output bytes — the compressor supports this via a direct zero-count
  check before the hash chain walk

### Checksum Summary

| Location | Algorithm |
|---|---|
| File header checksum (list header `[0:4]`) | Sum of LE uint32 words from bytes `[4:]` of list header + entry records |
| List entry checksum (`entry_record[4:8]`) | Adler32 of Feistel-encrypted entry blob |
| Entry header checksum (`entry_hdr[40:44]`) | Adler32 of every 16th byte of decompressed chunk data |
| Module entry DataCk (`mod_entry[36:40]`) | Adler32 of Feistel-decrypted partition data (non-MAIN only) |
| Module entry EntryCk (`mod_entry[44:48]`) | Adler32 of module entry bytes `[0:44]` |

### Cramfs Format

Panasonic uses "old cramfs format" (flags=0, no `FSID_VERSION_2`):
- 4 KB block size
- zlib compression with raw deflate (`wbits=-14`)
- Symlinks stored as compressed data (same as regular files)

---

## PTY / Interactive Shell

The device has devpts kernel support but it is not mounted by default. To enable
interactive shells (e.g. via dropbear SSH):

```bash
mkdir -p /dev/pts
mount -t devpts devpts /dev/pts
```

After this, PTY allocation works correctly and interactive terminal sessions are fully
functional.

---

## Build Instructions

### Requirements

- Go 1.21 or later

### Building

```bash
make build      # Build all binaries (panago + panago-cli)
make test       # Run tests
make clean      # Clean build artifacts
```

### Cross-compilation

```bash
# For ARM devices
GOOS=linux GOARCH=arm go build -o panago-arm ./cmd/cli
```

### Releases

```bash
# Local test build
goreleaser release --snapshot --skip=publish --clean

# Tag and push to trigger official release
git tag v0.1.2
git push origin v0.1.2
```

### Integration Tests

Firmware integration tests require a real firmware file. Set the path via environment
variable or place the file at the default location:

```bash
# Use a specific firmware file
PANAGO_TEST_FIRMWARE=/path/to/PANAEUSB.FRM go test ./pkg/firmware/

# Run only unit tests (no firmware required)
go test ./pkg/firmware/ -run TestLZSS
```

---

## License

See [LICENSE](LICENSE) file.
