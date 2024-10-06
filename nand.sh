cat << EOF | bash
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
EOF
