# panago

Golang toolkit for Panasonic DP-UB9000 (and similar) UHD Blu-ray player research and modification.

## Features

- **Firmware Tools** - Decode, encode, and analyze Panasonic firmware files (PANAEUSB.FRM)
- **Cramfs Tools** - Extract and create cramfs filesystem images used by Panasonic devices
- **Device Discovery** - Find Panasonic players on the network
- **Shell Access** - Interactive shell for exploited devices
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

## Usage

### Unified CLI (`panago-cli`)

The unified CLI provides all firmware and cramfs tools in a single binary:

```bash
panago-cli [command] [subcommand] [options]
```

### Firmware Tools

#### Show Firmware Information

```bash
panago-cli firmware info PANAEUSB.FRM
```

Output:
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

#### Extract Firmware

```bash
panago-cli firmware decode PANAEUSB.FRM ./extracted/
```

This extracts all partitions:
- `PROG_0.00.bin` - Boot program
- `MAIN.bin` - Main filesystem (concatenated cramfs images)
- `MINI_7.74.bin` - Mini partition
- `DRV1_D110.bin` - Driver partition 1
- `DRV1_V304.bin` - Driver partition 2
- `BUCD_000.bin` - BD certification data

#### Rebuild Firmware

```bash
panago-cli firmware encode ./modified/ output.FRM original.FRM
```

Uses the original firmware as a template and replaces partitions with modified versions from the input directory. Partition files must match the naming convention `NAME_VERSION.bin`.

#### Split MAIN.bin

MAIN.bin contains concatenated sub-images. Split them for individual modification:

```bash
panago-cli firmware split-main MAIN.bin ./main_parts/
```

Output:
```
Splitting MAIN.bin...
  fma4: offset=0x0, size=6553600 (raw)
  fma5: offset=0x640000, size=47710208 (cramfs)
  fma6: offset=0x33c0000, size=18874368 (romfs)
  fma7: offset=0x45c0000, size=111411200 (cramfs)
```

The sub-images are:
- `fma4.bin` - Kernel (raw binary)
- `fma5.bin` - Root filesystem (cramfs)
- `fma6.bin` - Data partition (romfs)
- `fma7.bin` - Application filesystem (cramfs)

#### Combine MAIN.bin

After modifying sub-images, combine them back:

```bash
panago-cli firmware combine-main ./main_parts/ MAIN_modified.bin
```

#### Test Crypto Implementations

```bash
panago-cli firmware test
```

Verifies AES-128-CBC, Feistel cipher, and LZSS compression round-trips.

### Cramfs Tools

#### List Files in Cramfs Image

```bash
panago-cli cramfs list fma5.bin
```

Output:
```
d0755     0 usr
d0755     0 tmp
l0777     1 armv4t -> .
d0755   296 sbin
-0755 48060 sbin/init
l0777     3 var -> tmp
...
```

#### Extract Cramfs Image

```bash
panago-cli cramfs extract fma5.bin ./rootfs/
```

Extracts all files, directories, and symlinks preserving permissions.

#### Create Cramfs Image

```bash
panago-cli cramfs create ./rootfs/ new_fma5.bin
```

Creates a cramfs image compatible with Panasonic's "old cramfs format" (flags=0).

#### Show Cramfs Information

```bash
panago-cli cramfs info fma5.bin
```

Output:
```
Cramfs image: fma5.bin
Files: 171
Directories: 78
Symlinks: 156
Total uncompressed size: 38651830 bytes
```

### Romfs Tools

Romfs is used for the fma6 data partition. No root access required.

#### List Files in Romfs Image

```bash
panago-cli romfs list fma6.bin
```

#### Extract Romfs Image

```bash
panago-cli romfs extract fma6.bin ./data/
```

#### Create Romfs Image

```bash
panago-cli romfs create ./data/ new_fma6.bin
```

#### Show Romfs Information

```bash
panago-cli romfs info fma6.bin
```

Output:
```
Romfs image: fma6.bin
Volume name: rom 665d5f54
Image size: 18641888 bytes
Files: 666
Directories: 36
Symlinks: 35
Total content size: 18601986 bytes
```

### Standalone Tools

Individual tools are also available:

```bash
# Firmware tool
./bin/firmware decode PANAEUSB.FRM ./extracted/

# Cramfs tool
./bin/cramfsck extract-all fma5.bin ./rootfs/
```

### TUI Application

The graphical terminal interface for device management:

```bash
./bin/panago
```

## Firmware Modification Workflow

### Simple Workflow (Recommended)

The easiest way to modify firmware using just two commands:

```bash
# 1. Extract everything
panago-cli extract PANAEUSB.FRM ./workspace/

# 2. Modify files in the workspace
#    - ./workspace/fma5/ = root filesystem (contains /sbin/init, /etc/, etc.)
#    - ./workspace/fma6/ = data partition (fonts, pixmaps, etc.)
#    - ./workspace/fma7/ = app filesystem (application binaries, libraries)

# 3. Build new firmware
panago-cli build ./workspace/ PANAEUSB_modified.FRM PANAEUSB.FRM
```

The workspace structure after extraction:
```
workspace/
├── PROG_*.bin, MINI_*.bin, DRV1_*.bin, BUCD_*.bin  (other partitions)
├── fma4.bin      (kernel - kept as binary)
├── fma5/         (root filesystem - editable)
│   ├── sbin/init
│   ├── etc/
│   └── ...
├── fma6/         (data partition - editable)
│   ├── local/fonts/
│   └── ...
└── fma7/         (app filesystem - editable)
    ├── bin/
    ├── dtvrec/
    └── ...
```

### Advanced Workflow

For more control, use individual commands:

```bash
# Extract firmware partitions
panago-cli firmware decode PANAEUSB.FRM ./extracted/

# Split MAIN.bin into sub-images
panago-cli firmware split-main ./extracted/MAIN.bin ./main_parts/

# Extract specific filesystem
panago-cli cramfs extract ./main_parts/fma5.bin ./rootfs/
panago-cli romfs extract ./main_parts/fma6.bin ./data/
panago-cli cramfs extract ./main_parts/fma7.bin ./appfs/

# Make modifications...

# Rebuild filesystems
panago-cli cramfs create ./rootfs/ ./main_parts/fma5.bin
panago-cli romfs create ./data/ ./main_parts/fma6.bin
panago-cli cramfs create ./appfs/ ./main_parts/fma7.bin

# Combine and encode
panago-cli firmware combine-main ./main_parts/ ./extracted/MAIN.bin
panago-cli firmware encode ./extracted/ PANAEUSB_modified.FRM PANAEUSB.FRM
```

## Technical Details

### Firmware Encryption

Panasonic firmware uses a two-layer encryption scheme:

1. **AES-128-CBC** - Applied to the entire file (outer layer)
2. **Custom Feistel Cipher** - 16-round, 8-byte blocks with custom S-box
   - Applied to header and partition table
   - Applied to first/last 5KB of each MAIN sub-entry

### Cramfs Format

Panasonic uses "old cramfs format" (flags=0, no FSID_VERSION_2):
- 4KB block size
- zlib compression with raw deflate (wbits=-14)
- Symlinks stored as compressed data (same as regular files)

## Build Instructions

### Requirements

- Go 1.21 or later

### Building

```bash
make build      # Build all binaries
make test       # Run tests
make clean      # Clean build artifacts
```

### Cross-compilation

```bash
# For ARM devices
GOOS=linux GOARCH=arm go build -o panago-arm ./cmd/cli
```

### Release

Edit `.goreleaser.yaml` for build targets, then:

```bash
# Local test build
goreleaser release --snapshot --skip=publish --clean

# Official release (triggers on git tag)
git tag v0.1.2
git push origin v0.1.2
```

## Enabling Interactive Shell (PTY Support)

The device has devpts kernel support but it's not mounted by default. This is why interactive shells don't work out of the box.

**To enable proper PTY/terminal support:**

```bash
mount -t devpts devpts /dev/pts
```

After this, you can:
- Use interactive shells via SSH (dropbear)
- Spawn proper terminal sessions
- Run programs that require a TTY

**Note:** The punch/shell workarounds in panago were created before discovering this. With devpts mounted, a simple dropbear SSH server provides full interactive access.

## Quick Setup Script

After gaining shell access, run:

```bash
# Enable PTY support
mkdir -p /dev/pts
mount -t devpts devpts /dev/pts

# Now interactive shells work
```

## License

See [LICENSE](LICENSE) file.
