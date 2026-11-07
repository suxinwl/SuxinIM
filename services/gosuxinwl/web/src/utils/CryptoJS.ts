import CryptoJS from 'crypto-js'
const aesKey= CryptoJS.enc.Utf8.parse('eI2lI3zL0qD0hY7m') 
const aesIv=  CryptoJS.enc.Utf8.parse('6289984834922396')
/* 加密 */
function Encrypt (word: any) {
  const srcs = CryptoJS.enc.Utf8.parse(word)
  const encrypted = CryptoJS.AES.encrypt(srcs, aesKey, { iv: aesIv, mode: CryptoJS.mode.CBC, padding: CryptoJS.pad.Pkcs7 })
  return CryptoJS.enc.Base64.stringify(encrypted.ciphertext)
}
/* 解密 */
function Decrypt (word: any) {
  const decrypt = CryptoJS.AES.decrypt(word, aesKey, { iv: aesIv, mode: CryptoJS.mode.CBC, padding: CryptoJS.pad.Pkcs7 })
  const decryptedStr = decrypt.toString(CryptoJS.enc.Utf8)
  return decryptedStr.toString()
}
export { Encrypt,Decrypt };