#!/bin/bash

# Execute the script using a Here Document, feeding the commands to bash
cat << 'EOF' | bash

# Check if /dev/fd is a symbolic link, remove it if it exists
if [ -L "/dev/fd" ]; then
    rm /dev/fd
fi

# Create a symbolic link from /proc/self/fd to /dev/fd
ln -s /proc/self/fd /dev/fd

# Set variables for comment and timestamp
comment="{comment}"
timestamp="{time}"
backup_path="/mnt/usb/storage/backup/eeprom/$timestamp"

# Print header for backup process
echo -e "\n===================================================================================="
echo -e "=====================[ usb://backup/eeprom/$timestamp ]======================="
echo -e "====================================================================================\n"

# Retrieve firmware version from version file
fwversion=$(cat /usr/target/version.txt)

# Create the backup directory and navigate to it
mkdir -p "$backup_path"
cd "$backup_path"

# Save the environment variables into a compressed file
printenv | gzip -c > env.gz

# Start backing up the EEPROM
echo "Start :: EEPROM [ 256 Kb ]"

# Use dd to read from EEPROM, compress the output, and calculate the checksum
# Progress is shown and output is redirected to both gzip and sha1sum
# "stdbuf" ensures consistent line buffering for progress display
# Output is saved as eeprom.img.gz and checksum is saved in eeprom.checksum

dd if=/conf/fake_eeprom status=progress conv=notrunc 2> >(stdbuf -oL -eL tr '\r' '\n') \
   1> >(tee >(gzip -c > eeprom.img.gz) | sha1sum -b > eeprom.checksum)

# Sync to ensure data is written to disk
sync

# Extract the checksum from the checksum file
checksum=$(awk '{print $1}' eeprom.checksum)

# Create metadata for the backup
cat << META > eeprom.meta
comment=$comment
device=/dev/${fma}
size_k=${size}
sha1=${checksum}
date=${timestamp}
product=${PRODUCT}
fwversion=${fwversion}
META

# Print completion message for EEPROM backup
echo -e "\nEnd :: EEPROM [ $checksum ]"

# Clean up by removing the checksum file
rm eeprom.checksum

# Remove /dev/fd symbolic link
rm /dev/fd

# Sync to ensure all changes are committed
echo -e "\n==============================[ Backup Completed ]==================================\n"
sync
echo "##DONE##"
EOF

# Repeat the above process for NAND backup
cat << 'EOF' | bash

# Check if /dev/fd is a symbolic link, remove it if it exists
if [ -L "/dev/fd" ]; then
    rm /dev/fd
fi

# Create a symbolic link from /proc/self/fd to /dev/fd
ln -s /proc/self/fd /dev/fd

# Set variables for comment and timestamp
comment="{comment}"
timestamp="{time}"
backup_path="/mnt/usb/storage/backup/nand/$timestamp"

# Print header for backup process
echo -e "\n===================================================================================="
echo -e "======================[ usb://backup/nand/$timestamp ]========================"
echo -e "====================================================================================\n"

# Retrieve firmware version from version file
fwversion=$(cat /usr/target/version.txt)

# Create the backup directory and navigate to it
mkdir -p "$backup_path"
cd "$backup_path"

# Save the environment variables into a compressed file
printenv | gzip -c > env.gz

# Find the NAND partitions and their sizes
sizes=$(grep -E 'fma(2|3|4|5|6|7|8|9|10|11|12)' /proc/driver/nand | awk '{print $4}')

# Iterate over each NAND partition size and back up the data
for size in $sizes; do
    # Extract the partition name for the current size
    fma=$(grep -E 'fma(2|3|4|5|6|7|8|9|10|11|12)' /proc/driver/nand | grep -F "$size" | awk '{print $1}')
    # Convert size from KB to MB for display
    sizemb=$(echo "scale=2; $size / 1024" | bc)

    # Start backing up the NAND partition
    echo "Start :: /dev/$fma [ $sizemb Mb ]"

    # Use dd to read from NAND, compress the output, and calculate the checksum
    dd if=/dev/$fma bs=1024 count=$size status=progress conv=notrunc 2> >(stdbuf -oL -eL tr '\r' '\n') \
       1> >(tee >(gzip -c > ${fma}.img.gz) | sha1sum -b > ${fma}.checksum)

    # Sync to ensure data is written to disk
    sync

    # Extract the checksum from the checksum file
    checksum=$(awk '{print $1}' ${fma}.checksum)

    # Create metadata for the backup
    cat << META > ${fma}.meta
comment=$comment
device=/dev/${fma}
size_k=${size}
sha1=${checksum}
date=${timestamp}
product=${PRODUCT}
fwversion=${fwversion}
META

    # Print completion message for the current NAND partition
    echo -e "\nEnd :: /dev/$fma [ $checksum ]"

    # Clean up by removing the checksum file
    rm ${fma}.checksum

    # Print separator line if not the last partition
    if [[ "$fma" != "fma11" ]]; then
        echo -e "\n====================================================================================\n"
    fi
done

# Remove /dev/fd symbolic link
rm /dev/fd

# Sync to ensure all changes are committed
sync

# Print completion message for NAND backup
echo -e "\n================================[ Backup Completed ]================================\n"
echo "##DONE##"
EOF
