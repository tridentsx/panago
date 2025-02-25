# Using Different Endianness in cramfs

The cramfs package now supports both little-endian and big-endian formats. Here's how to use it:

```go
// For little-endian (default)
cfg := cramfs.DefaultConfig()  // Uses binary.LittleEndian by default

// For big-endian
cfg := &cramfs.Config{
    Endianness: binary.BigEndian,
}

// Example usage with functions
err := cramfs.ListFiles("image.cramfs", cfg)
err := cramfs.ExtractAll("image.cramfs", "output/", cfg)
err := cramfs.CompressToCramfs("input/", "output.cramfs", cfg)
```

The endianness setting affects how all binary data is read from and written to the cramfs image, including:
- Superblock structure
- Inode structures
- File offsets and sizes

By default, if no config is provided (passing nil), the functions will use little-endian encoding.