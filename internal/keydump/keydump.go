package keydump

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const (
	punchPort       = 2222
	libPath         = "/usr/lib/libfmupre.so"
	libBackupPath   = "/usr/lib/libfmupre.so.bak"
	keyDumpPath     = "/tmp/fpc_keys.bin"
	timeout         = 5 * time.Second
)

// KeyDump handles FPC key extraction from Panasonic players
type KeyDump struct {
	conn     net.Conn
	targetIP string
}

// New creates a new KeyDump session
func New(ip string) (*KeyDump, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, punchPort), timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	return &KeyDump{conn: conn, targetIP: ip}, nil
}

// Close closes the session
func (k *KeyDump) Close() error {
	if k.conn != nil {
		return k.conn.Close()
	}
	return nil
}

// exec sends a command and returns the response
func (k *KeyDump) exec(cmd string) (string, error) {
	if _, err := k.conn.Write([]byte(cmd + "\n")); err != nil {
		return "", err
	}
	
	buf := make([]byte, 4096)
	k.conn.SetReadDeadline(time.Now().Add(timeout))
	n, err := k.conn.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
}

// BackupOriginal backs up the original libfmupre.so
func (k *KeyDump) BackupOriginal() error {
	_, err := k.exec(fmt.Sprintf("cp %s %s", libPath, libBackupPath))
	return err
}

// RestoreOriginal restores the original libfmupre.so
func (k *KeyDump) RestoreOriginal() error {
	_, err := k.exec(fmt.Sprintf("cp %s %s", libBackupPath, libPath))
	return err
}

// UploadPatchedLib uploads the patched library to the device
func (k *KeyDump) UploadPatchedLib(patchedLibPath string) error {
	// Read patched library
	data, err := os.ReadFile(patchedLibPath)
	if err != nil {
		return fmt.Errorf("failed to read patched lib: %w", err)
	}

	// Upload via base64 encoding (shell-safe)
	// Split into chunks to avoid command line limits
	chunkSize := 4096
	tmpPath := "/tmp/libfmupre_patched.so"
	
	// Remove any existing file
	k.exec(fmt.Sprintf("rm -f %s", tmpPath))
	
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		
		// Use printf with hex escape sequences
		cmd := fmt.Sprintf("printf '%s' >> %s", hexToEscaped(chunk), tmpPath)
		if _, err := k.exec(cmd); err != nil {
			return fmt.Errorf("failed to upload chunk: %w", err)
		}
	}
	
	// Verify size
	resp, err := k.exec(fmt.Sprintf("stat -c %%s %s", tmpPath))
	if err != nil {
		return fmt.Errorf("failed to verify upload: %w", err)
	}
	
	fmt.Printf("Uploaded %d bytes, device reports: %s", len(data), resp)
	
	// Copy to target location
	_, err = k.exec(fmt.Sprintf("cp %s %s && chmod 755 %s", tmpPath, libPath, libPath))
	return err
}

// hexToEscaped converts bytes to shell-escaped format
func hexToEscaped(data []byte) string {
	var buf bytes.Buffer
	for _, b := range data {
		buf.WriteString(fmt.Sprintf("\\x%02x", b))
	}
	return buf.String()
}

// TriggerFirmwareUpdate triggers a firmware update to capture keys
func (k *KeyDump) TriggerFirmwareUpdate() error {
	// The firmware update process will call the patched libfmupre.so
	// which will dump keys to /tmp/fpc_keys.bin
	
	// Note: This requires a USB drive with firmware to be inserted
	// or we can trigger a partial update that just initializes the keys
	
	_, err := k.exec("fmupre_test 2>/dev/null || true")
	return err
}

// ReadKeys reads the dumped keys from the device
func (k *KeyDump) ReadKeys() (k1, k2 []byte, err error) {
	// Check if key file exists
	resp, err := k.exec(fmt.Sprintf("test -f %s && echo EXISTS", keyDumpPath))
	if err != nil || resp != "EXISTS\n" {
		return nil, nil, fmt.Errorf("key file not found - firmware update may not have run")
	}
	
	// Read key file as hex
	resp, err = k.exec(fmt.Sprintf("xxd -p %s | tr -d '\\n'", keyDumpPath))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read keys: %w", err)
	}
	
	keys, err := hex.DecodeString(resp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode keys: %w", err)
	}
	
	if len(keys) != 16 {
		return nil, nil, fmt.Errorf("unexpected key length: %d", len(keys))
	}
	
	return keys[:8], keys[8:], nil
}

// ExtractKeys performs the full key extraction process
func (k *KeyDump) ExtractKeys(patchedLibPath string) (k1, k2 []byte, err error) {
	fmt.Println("[*] Backing up original libfmupre.so...")
	if err := k.BackupOriginal(); err != nil {
		return nil, nil, fmt.Errorf("backup failed: %w", err)
	}
	
	fmt.Println("[*] Uploading patched library...")
	if err := k.UploadPatchedLib(patchedLibPath); err != nil {
		k.RestoreOriginal()
		return nil, nil, fmt.Errorf("upload failed: %w", err)
	}
	
	fmt.Println("[*] Triggering firmware update...")
	fmt.Println("    (Insert USB with firmware and wait for update to start)")
	
	// Wait for keys to be dumped
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		k1, k2, err = k.ReadKeys()
		if err == nil {
			fmt.Println("[+] Keys extracted successfully!")
			break
		}
	}
	
	fmt.Println("[*] Restoring original library...")
	k.RestoreOriginal()
	
	if err != nil {
		return nil, nil, fmt.Errorf("key extraction failed: %w", err)
	}
	
	return k1, k2, nil
}
