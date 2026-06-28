package main

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"testing"
	"time"
)

// ==================== obfsBuildNonce ====================

func TestObfsBuildNonce(t *testing.T) {
	nonce := obfsBuildNonce(0xDEADBEEF, 0xCAFE, 0x12345678)
	if len(nonce) != 12 {
		t.Fatalf("ожидал 12 байт, получил %d", len(nonce))
	}

	if uint32(nonce[0])<<24|uint32(nonce[1])<<16|uint32(nonce[2])<<8|uint32(nonce[3]) != 0xDEADBEEF {
		t.Error("SSRC не совпадает")
	}
	if uint16(nonce[4])<<8|uint16(nonce[5]) != 0xCAFE {
		t.Error("Seq не совпадает")
	}
	if nonce[6] != 0 || nonce[7] != 0 {
		t.Error("байты 6-7 должны быть 0")
	}
	if uint32(nonce[8])<<24|uint32(nonce[9])<<16|uint32(nonce[10])<<8|uint32(nonce[11]) != 0x12345678 {
		t.Error("TS не совпадает")
	}
}

// ==================== NewObfsConfig / NewObfsState ====================

func TestNewObfsConfig(t *testing.T) {
	cfg := NewObfsConfig()
	if cfg == nil {
		t.Fatal("NewObfsConfig вернул nil")
	}
	if cfg.PayloadType != 111 {
		t.Errorf("ожидал PayloadType 111, получил %d", cfg.PayloadType)
	}
	if cfg.PaddingMax != 255 {
		t.Errorf("ожидал PaddingMax 255, получил %d", cfg.PaddingMax)
	}
}

func TestNewObfsState(t *testing.T) {
	state := NewObfsState()
	if state == nil {
		t.Fatal("NewObfsState вернул nil")
	}
}

// ==================== obfsWrapPacket / obfsUnwrapPacket ====================

func TestObfsRoundtrip(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	cfg := NewObfsConfig()
	state := NewObfsState()

	payload := []byte("Hello, WDTT! This is a test payload.")

	wrapped, err := obfsWrapPacket(key, payload, cfg, state)
	if err != nil {
		t.Fatalf("obfsWrapPacket ошибка: %v", err)
	}

	if len(wrapped) < 13 {
		t.Fatalf("wrapped packet слишком короткий: %d", len(wrapped))
	}

	if (wrapped[0] >> 6) != 2 {
		t.Error("wrapped packet не является RTP v2")
	}

	pt := wrapped[1] & 0x7F
	if pt != cfg.PayloadType {
		t.Errorf("payload type не совпадает: %d != %d", pt, cfg.PayloadType)
	}

	dst := make([]byte, 1600)
	n, err := obfsUnwrapPacket(key, wrapped, dst)
	if err != nil {
		t.Fatalf("obfsUnwrapPacket ошибка: %v", err)
	}

	unwrapped := dst[:n]
	if string(unwrapped) != string(payload) {
		t.Errorf("данные не совпадают: %q != %q", unwrapped, payload)
	}
}

func TestObfsEmptyPayload(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	cfg := NewObfsConfig()
	state := NewObfsState()

	_, err := obfsWrapPacket(key, []byte{}, cfg, state)
	if err == nil {
		t.Error("ожидал ошибку для пустого payload")
	}
}

func TestObfsSequentialPackets(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	cfg := NewObfsConfig()
	state := NewObfsState()

	payloadTemplate := []byte("packet-000")

	for i := 0; i < 100; i++ {
		payload := make([]byte, len(payloadTemplate))
		copy(payload, payloadTemplate)
		payload[len(payload)-1] = byte('0' + i%10)

		wrapped, err := obfsWrapPacket(key, payload, cfg, state)
		if err != nil {
			t.Fatalf("wrap packet %d ошибка: %v", i, err)
		}

		dst := make([]byte, 1600)
		n, err := obfsUnwrapPacket(key, wrapped, dst)
		if err != nil {
			t.Fatalf("unwrap packet %d ошибка: %v", i, err)
		}

		if string(dst[:n]) != string(payload) {
			t.Errorf("packet %d: данные не совпадают", i)
		}
	}
}

func TestObfsWrongKey(t *testing.T) {
	key1 := make([]byte, wrapKeyLen)
	key2 := make([]byte, wrapKeyLen)
	rand.Read(key1)
	rand.Read(key2)

	cfg := NewObfsConfig()
	state := NewObfsState()

	payload := []byte("secret data")

	wrapped, err := obfsWrapPacket(key1, payload, cfg, state)
	if err != nil {
		t.Fatalf("wrap ошибка: %v", err)
	}

	dst := make([]byte, 1600)
	_, err = obfsUnwrapPacket(key2, wrapped, dst)
	if err == nil {
		t.Error("ожидал ошибку аутентификации при неверном ключе")
	}
}

func TestObfsInvalidKeyLength(t *testing.T) {
	payload := []byte("test")
	cfg := NewObfsConfig()
	state := NewObfsState()

	_, err := obfsWrapPacket([]byte("short"), payload, cfg, state)
	if err == nil {
		t.Error("ожидал ошибку при коротком ключе")
	}

	// 33 bytes
	longKey := make([]byte, 33)
	rand.Read(longKey)
	_, err = obfsWrapPacket(longKey, payload, cfg, state)
	if err == nil {
		t.Error("ожидал ошибку при длинном ключе")
	}

	dst := make([]byte, 1600)
	_, err = obfsUnwrapPacket([]byte("short"), []byte("some-packet-data"), dst)
	if err == nil {
		t.Error("ожидал ошибку при коротком ключе для unwrap")
	}
}

func TestObfsShortPacket(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	dst := make([]byte, 1600)

	_, err := obfsUnwrapPacket(key, []byte("short"), dst)
	if err == nil {
		t.Error("ожидал ошибку для короткого пакета")
	}

	_, err = obfsUnwrapPacket(key, []byte{}, dst)
	if err == nil {
		t.Error("ожидал ошибку для пустого пакета")
	}
}

func TestObfsTamperedPacket(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	cfg := NewObfsConfig()
	state := NewObfsState()
	payload := []byte("important data")

	wrapped, err := obfsWrapPacket(key, payload, cfg, state)
	if err != nil {
		t.Fatalf("wrap ошибка: %v", err)
	}

	tampered := make([]byte, len(wrapped))
	copy(tampered, wrapped)
	tampered[15] ^= 0xFF

	dst := make([]byte, 1600)
	_, err = obfsUnwrapPacket(key, tampered, dst)
	if err == nil {
		t.Error("ожидал ошибку для повреждённого пакета")
	}
}

// ==================== obfsIsRTPPacket ====================

func TestObfsIsRTPPacket(t *testing.T) {
	if obfsIsRTPPacket(nil) {
		t.Error("nil не должен быть RTP")
	}
	if obfsIsRTPPacket([]byte{}) {
		t.Error("пустой пакет не должен быть RTP")
	}
	if obfsIsRTPPacket([]byte{0x80, 0x00}) {
		t.Error("короткий пакет не должен быть RTP")
	}

	rtp := []byte{0x80, 0x6F, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !obfsIsRTPPacket(rtp) {
		t.Error("RTP v2 с PT=111 должен распознаваться")
	}

	notRTP := []byte{0x80, 0x50, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if obfsIsRTPPacket(notRTP) {
		t.Error("пакет с PT=80 не должен распознаваться как RTP")
	}
}

// ==================== getAEAD / cacheKeyForAEAD ====================

func TestCacheKeyForAEAD(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	key1 := cacheKeyForAEAD(key)
	if key1 == "" {
		t.Error("cacheKeyForAEAD вернул пустую строку")
	}

	key2 := cacheKeyForAEAD(key)
	if key1 != key2 {
		t.Error("cacheKeyForAEAD недетерминирован")
	}

	if strings.Contains(key1, string(key)) {
		t.Error("cacheKeyForAEAD содержит исходный ключ")
	}

	if len(key1) != 64 { // SHA256 hex = 64 символа
		t.Errorf("ожидал 64 символа, получил %d", len(key1))
	}
}

func TestGetAEAD(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	aead1, err := getAEAD(key)
	if err != nil {
		t.Fatalf("getAEAD ошибка: %v", err)
	}

	aead2, err := getAEAD(key)
	if err != nil {
		t.Fatalf("getAEAD (2) ошибка: %v", err)
	}

	if aead1 != aead2 {
		t.Error("getAEAD не кеширует результаты")
	}

	_, err = getAEAD([]byte("short"))
	if err == nil {
		t.Error("ожидал ошибку при коротком ключе")
	}
}

// ==================== zeroBytes ====================

func TestZeroBytes(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5}
	zeroBytes(b)
	for i, v := range b {
		if v != 0 {
			t.Errorf("byte[%d] = %d, ожидал 0", i, v)
		}
	}

	zeroBytes(nil)
}

// ==================== deriveWrapKey ====================

func TestDeriveWrapKey(t *testing.T) {
	key, err := deriveWrapKey("test-password")
	if err != nil {
		t.Fatalf("deriveWrapKey ошибка: %v", err)
	}
	if len(key) != wrapKeyLen {
		t.Errorf("длина ключа: %d, ожидал %d", len(key), wrapKeyLen)
	}

	key2, _ := deriveWrapKey("test-password")
	if !bytesEqual(key, key2) {
		t.Error("deriveWrapKey недетерминирован")
	}

	key3, _ := deriveWrapKey("different-password")
	if bytesEqual(key, key3) {
		t.Error("разные пароли дали одинаковые ключи")
	}

	_, err = deriveWrapKey("")
	if err == nil {
		t.Error("ожидал ошибку для пустого пароля")
	}
}

// ==================== wrapKeyStore ====================

func TestWrapKeyStoreSetPasswords(t *testing.T) {
	store := newWrapKeyStore()

	err := store.SetPasswords("main-pass", []string{"gen1", "gen2"})
	if err != nil {
		t.Fatalf("SetPasswords ошибка: %v", err)
	}

	if store.Count() != 3 {
		t.Errorf("ожидал 3 ключа, получил %d", store.Count())
	}

	err = store.SetPasswords("new-main", []string{"gen3"})
	if err != nil {
		t.Fatalf("SetPasswords (2) ошибка: %v", err)
	}

	if store.Count() != 2 {
		t.Errorf("ожидал 2 ключа после обновления, получил %d", store.Count())
	}
}

func TestWrapKeyStoreAddPassword(t *testing.T) {
	store := newWrapKeyStore()

	err := store.AddPassword("test-password")
	if err != nil {
		t.Fatalf("AddPassword ошибка: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("ожидал 1 ключ, получил %d", store.Count())
	}

	err = store.AddPassword("test-password")
	if err != nil {
		t.Fatalf("AddPassword (дубль) ошибка: %v", err)
	}

	if store.Count() != 1 {
		t.Errorf("ожидал 1 ключ после дубля, получил %d", store.Count())
	}
}

func TestWrapKeyStoreEmptyPassword(t *testing.T) {
	store := newWrapKeyStore()

	err := store.AddPassword("")
	if err == nil {
		t.Error("ожидал ошибку для пустого пароля")
	}
}

// ==================== wrapKeyID ====================

func TestWrapKeyID(t *testing.T) {
	id1 := wrapKeyID("password123")
	if len(id1) != 16 {
		t.Errorf("ожидал 16 символов, получил %d", len(id1))
	}

	id2 := wrapKeyID("password123")
	if id1 != id2 {
		t.Error("wrapKeyID недетерминирован")
	}

	id3 := wrapKeyID("different")
	if id1 == id3 {
		t.Error("разные пароли дали одинаковые ID")
	}
}

// ==================== generatePassword ====================

func TestGeneratePassword(t *testing.T) {
	pw := generatePassword()
	if len(pw) != generatedPasswordLen {
		t.Errorf("ожидал %d символов, получил %d", generatedPasswordLen, len(pw))
	}

	for _, c := range pw {
		if !strings.ContainsRune(passChars, c) {
			t.Errorf("недопустимый символ %c в пароле %q", c, pw)
		}
	}

	if pw == "" {
		t.Error("пароль пустой")
	}
}

// ==================== maskPassword ====================

func TestMaskPassword(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"a", "a"},
		{"ab", "ab"},
		{"abc", "abc"},
		{"abcd", "abc****"},
		{"abcdefgh", "abc****"},
		{"abcdefghij", "abc****"},
	}

	for _, tc := range tests {
		got := maskPassword(tc.input)
		if got != tc.want {
			t.Errorf("maskPassword(%q) = %q, ожидал %q", tc.input, got, tc.want)
		}
	}
}

// ==================== isPasswordExpired ====================

func TestIsPasswordExpired(t *testing.T) {
	now := time.Now()

	active := &PasswordEntry{ExpiresAt: now.Add(time.Hour).Unix()}
	if isPasswordExpired(active) {
		t.Error("пароль с истечением через час не должен быть истёкшим")
	}

	expired := &PasswordEntry{ExpiresAt: now.Add(-time.Hour).Unix()}
	if !isPasswordExpired(expired) {
		t.Error("пароль с истечением час назад должен быть истёкшим")
	}

	eternal := &PasswordEntry{ExpiresAt: 0}
	if isPasswordExpired(eternal) {
		t.Error("пароль с ExpiresAt=0 не должен быть истёкшим")
	}
}

// ==================== stripVkUrl ====================

func TestStripVkUrl(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"https://vk.com/durov", "durov"},
		{"  https://vk.com/durov  ", "durov"},
		{"https://vk.com/durov?from=groups", "durov"},
		{"https://vk.com/club123456?ref=some", "club123456"},
		{"id123456", "id123456"},
		{"  id123456  ", "id123456"},
	}

	for _, tc := range tests {
		got := stripVkUrl(tc.input)
		if got != tc.want {
			t.Errorf("stripVkUrl(%q) = %q, ожидал %q", tc.input, got, tc.want)
		}
	}
}

// ==================== salamanderXOR ====================

func TestSalamanderXOROff(t *testing.T) {
	dst := make([]byte, 5)
	src := []byte("hello")
	nonce := []byte("test-nonce")

	salamanderXOR(dst, src, nonce)

	for i := range dst {
		if dst[i] != src[i] {
			t.Errorf("salamanderXOR без ключа изменил данные (byte %d)", i)
		}
	}
}

func TestSalamanderXORRoundtrip(t *testing.T) {
	oldKey := salamanderKey
	defer func() { salamanderKey = oldKey }()

	salamanderKey = make([]byte, 32)
	rand.Read(salamanderKey)

	src := []byte("secret-data")
	nonce := []byte("unique-nonce-123")

	encrypted := make([]byte, len(src))
	salamanderXOR(encrypted, src, nonce)

	decrypted := make([]byte, len(src))
	salamanderXOR(decrypted, encrypted, nonce)

	if string(decrypted) != string(src) {
		t.Errorf("salamander roundtrip: %q != %q", decrypted, src)
	}

	encrypted2 := make([]byte, len(src))
	salamanderXOR(encrypted2, src, []byte("different-nonce"))

	if bytesEqual(encrypted, encrypted2) {
		t.Error("разные nonce дали одинаковый шифротекст")
	}
}

func TestSalamanderXORLargeData(t *testing.T) {
	oldKey := salamanderKey
	defer func() { salamanderKey = oldKey }()

	salamanderKey = make([]byte, 32)
	rand.Read(salamanderKey)

	sizes := []int{0, 1, 100, 1024, 1500}
	for _, size := range sizes {
		src := make([]byte, size)
		rand.Read(src)

		dst := make([]byte, size)
		nonce := make([]byte, 8)
		rand.Read(nonce)

		salamanderXOR(dst, src, nonce)

		if size == 0 {
			continue
		}

		back := make([]byte, size)
		salamanderXOR(back, dst, nonce)

		if !bytesEqual(back, src) {
			t.Errorf("size %d: roundtrip не совпал", size)
		}
	}
}

// ==================== generateKeyPair ====================

func TestGenerateKeyPair(t *testing.T) {
	priv, pub, err := generateKeyPair()
	if err != nil {
		t.Fatalf("generateKeyPair ошибка: %v", err)
	}

	if priv == "" {
		t.Error("private key пуст")
	}
	if pub == "" {
		t.Error("public key пуст")
	}

	privBytes, err := base64.StdEncoding.DecodeString(priv)
	if err != nil || len(privBytes) != 32 {
		t.Error("private key невалидный base64 или длина != 32")
	}

	pubBytes, err := base64.StdEncoding.DecodeString(pub)
	if err != nil || len(pubBytes) != 32 {
		t.Error("public key невалидный base64 или длина != 32")
	}
}

// ==================== checkRateLimit ====================

func TestCheckRateLimit(t *testing.T) {
	addr := "10.0.0.1"

	for i := 0; i < authRateLimitMax; i++ {
		if !checkRateLimit(addr) {
			t.Fatalf("вызов %d превысил лимит", i+1)
		}
	}

	if checkRateLimit(addr) {
		t.Error("ожидал блокировку после превышения лимита")
	}
}

func TestCheckRateLimitDifferentAddrs(t *testing.T) {
	for i := 0; i < 10; i++ {
		addr := "192.168.99." + string(rune('1'+i))
		if !checkRateLimit(addr) {
			t.Errorf("неожиданная блокировка для %s", addr)
		}
	}
}

// ==================== getBuf / putBuf ====================

func TestBufPoolRoundtrip(t *testing.T) {
	b1 := getBuf()
	if b1 == nil {
		t.Fatal("getBuf вернул nil")
	}
	if len(*b1) != 1600 {
		t.Errorf("ожидал буфер 1600, получил %d", len(*b1))
	}

	(*b1)[0] = 0x42

	b2 := getBuf()
	putBuf(b1)
	putBuf(b2)
}

// ==================== concurrency test ====================

func TestObfsConcurrency(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	cfg := NewObfsConfig()
	state := NewObfsState()

	payload := []byte("concurrent-test")

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wrapped, err := obfsWrapPacket(key, payload, cfg, state)
			if err != nil {
				errCh <- err
				return
			}

			dst := make([]byte, 1600)
			n, err := obfsUnwrapPacket(key, wrapped, dst)
			if err != nil {
				errCh <- err
				return
			}

			if string(dst[:n]) != string(payload) {
				errCh <- nil // сигнал о несовпадении
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent ошибка: %v", err)
	}
}

func TestWrapKeyStoreConcurrent(t *testing.T) {
	store := newWrapKeyStore()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			password := "pass-" + string(rune('0'+idx))
			_ = store.AddPassword(password)
		}(i)
	}

	wg.Wait()

	if store.Count() < 1 || store.Count() > 10 {
		t.Errorf("неожиданное количество ключей: %d", store.Count())
	}
}

// ==================== aeadCache ====================

func TestAeadCacheClear(t *testing.T) {
	key := make([]byte, wrapKeyLen)
	rand.Read(key)

	aead1, _ := getAEAD(key)
	if aead1 == nil {
		t.Fatal("getAEAD вернул nil")
	}

	ck := cacheKeyForAEAD(key)
	aeadCache.Delete(ck)

	aead2, _ := getAEAD(key)
	if aead2 == nil {
		t.Fatal("getAEAD (2) вернул nil")
	}

	if aead1 == aead2 {
		t.Log("AEAD кеш вернул тот же объект после delete (возможно race)")
	}
}

func TestGetAEADKeyLength(t *testing.T) {
	tests := []struct {
		length int
		valid  bool
	}{
		{0, false},
		{16, false},
		{24, false},
		{31, false},
		{32, true},
		{33, false},
	}

	for _, tc := range tests {
		key := make([]byte, tc.length)
		_, err := getAEAD(key)
		if tc.valid && err != nil {
			t.Errorf("key length %d: ожидал success, получил %v", tc.length, err)
		}
		if !tc.valid && err == nil {
			t.Errorf("key length %d: ожидал ошибку", tc.length)
		}
	}
}

// ==================== buildClientConfig ====================

func TestBuildClientConfig(t *testing.T) {
	config := buildClientConfig("serverPubKey", "clientPrivKey", "10.66.66.2", "9000")

	if !strings.Contains(config, "serverPubKey") {
		t.Error("config не содержит serverPublic")
	}
	if !strings.Contains(config, "clientPrivKey") {
		t.Error("config не содержит clientPrivate")
	}
	if !strings.Contains(config, "10.66.66.2") {
		t.Error("config не содержит clientIP")
	}
	if !strings.Contains(config, "9000") {
		t.Error("config не содержит clientPort")
	}
	if !strings.Contains(config, "Endpoint") {
		t.Error("config не содержит Endpoint")
	}
}

// ==================== randomJitter ====================

func TestRandomJitterOff(t *testing.T) {
	old := enableJitter
	enableJitter = false
	defer func() { enableJitter = old }()

	start := time.Now()
	randomJitter()
	elapsed := time.Since(start)

	if elapsed > time.Millisecond {
		t.Errorf("jitter без флага не должен ждать: %v", elapsed)
	}
}

// ==================== getPublicIP ====================

func TestGetPublicIPInitial(t *testing.T) {
	old := publicIP
	publicIP = ""
	defer func() { publicIP = old }()

	ip := getPublicIP()
	if ip == "" {
		t.Log("getPublicIP вернул пустоту (возможно нет сети)")
	} else {
		t.Logf("getPublicIP = %s", ip)
	}
}

// ==================== глобальные константы ====================

func TestConstants(t *testing.T) {
	if authRateLimitMax <= 0 {
		t.Error("authRateLimitMax должен быть > 0")
	}
	if maxConcurrentClients <= 0 {
		t.Error("maxConcurrentClients должен быть > 0")
	}
	if wrapKeyLen != 32 {
		t.Errorf("wrapKeyLen ожидал 32, получил %d", wrapKeyLen)
	}
	if generatedPasswordLen != 16 {
		t.Errorf("generatedPasswordLen ожидал 16, получил %d", generatedPasswordLen)
	}
	if cap(connSemaphore) != maxConcurrentClients {
		t.Errorf("connSemaphore cap: %d != %d", cap(connSemaphore), maxConcurrentClients)
	}
}

// ==================== helper ====================

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
