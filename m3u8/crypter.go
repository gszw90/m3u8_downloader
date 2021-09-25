// package m3u8

package m3u8

import (
	"crypto/aes"
	"crypto/cipher"
)

// AesDecrypt aes解密
func AesDecrypt(cryptByte []byte, key []byte, iv []byte) (decryptBytes []byte, err error) {
	//生成block
	block, err := aes.NewCipher(key)
	if err != nil {
		return
	}
	blockSize := block.BlockSize()
	if len(iv) == 0 {
		iv = key
	}
	// 解密
	blockMode := cipher.NewCBCDecrypter(block, iv[:blockSize])
	decryptBytes = make([]byte, len(cryptByte))
	blockMode.CryptBlocks(decryptBytes, cryptByte)
	// 处理数据
	decryptBytes = PKCS7UnPadding(decryptBytes)
	return
}

// PKCS7UnPadding 去掉尾部多余的数据
func PKCS7UnPadding(data []byte) []byte {
	length := len(data)
	unpadding := int(data[length-1])
	return data[:(length - unpadding)]
}
