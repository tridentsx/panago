cat << EOF | bash
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
EOF
