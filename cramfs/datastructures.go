package cramfs

// Superblock represents the header of a Cramfs filesystem
// Added block alignment and padding for proper struct alignment
type Superblock struct {
	Magic     uint32
	Size      uint32
	Flags     uint32
	Future    uint32
	RootInode Inode
	Checksum  uint32
	Padding   [8]byte // Ensures proper alignment
}

// Inode represents an inode in the Cramfs filesystem
type Inode struct {
	Mode    uint16
	UID     uint16
	Size    uint32
	Offset  uint32
	GID     uint32
	Namelen uint16
}

type Inode struct {
	Mode    uint16 // File mode
	UID     uint16 // User ID
	Size    uint32 // File size
	GID     uint16 // Group ID
	Namelen uint16 // Length of name
	Offset  uint32 // Data block offset
}

/*
=====	=======================	=======================
0	ulelong	0x28cd3d45	Linux cramfs offset 0
>4	ulelong	x		size %d
>8	ulelong	x		flags 0x%x
>12	ulelong	x		future 0x%x
>16	string	>\0		signature "%.16s"
>32	ulelong	x		fsid.crc 0x%x
>36	ulelong	x		fsid.edition %d
>40	ulelong	x		fsid.blocks %d
>44	ulelong	x		fsid.files %d
>48	string	>\0		name "%.16s"
512	ulelong	0x28cd3d45	Linux cramfs offset 512
>516	ulelong	x		size %d
>520	ulelong	x		flags 0x%x
>524	ulelong	x		future 0x%x
>528	string	>\0		signature "%.16s"
>544	ulelong	x		fsid.crc 0x%x
>548	ulelong	x		fsid.edition %d
>552	ulelong	x		fsid.blocks %d
>556	ulelong	x		fsid.files %d
>560	string	>\0		name "%.16s"
=====	=======================	=======================
*/
