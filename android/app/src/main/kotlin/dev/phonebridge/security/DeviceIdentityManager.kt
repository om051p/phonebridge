package dev.phonebridge.security

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import org.json.JSONObject
import java.io.File
import java.nio.charset.StandardCharsets
import java.security.KeyStore
import java.security.PrivateKey
import java.security.PublicKey
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec

/**
 * Manages device Ed25519 identity on Android.
 *
 * Implements DEC-007 / DEC-011:
 * - Device identity is a 32-byte Ed25519 public key.
 * - Device ID is the 64-character lowercase hex SHA-256 fingerprint of the public key.
 * - Private key seed is encrypted at rest using Android Keystore AES-256-GCM where available.
 */
class DeviceIdentityManager(
    val deviceId: String,
    val displayName: String,
    val platform: String,
    val rawPublicKey: ByteArray,
    private val privateKey: PrivateKey
) {
    val publicKey: PublicKey = CryptoUtils.parsePublicKey(rawPublicKey)

    fun sign(message: ByteArray): ByteArray {
        return CryptoUtils.sign(privateKey, message)
    }

    companion object {
        private const val ANDROID_KEYSTORE = "AndroidKeyStore"
        private const val MASTER_KEY_ALIAS = "PhoneBridgeIdentityMasterKey"
        private const val GCM_TAG_LENGTH = 128
        private const val IDENTITY_FILE_NAME = "device_identity.json"

        @Synchronized
        fun loadOrGenerate(context: Context, defaultDisplayName: String = Build.MODEL ?: "Android Device"): DeviceIdentityManager {
            val storageDir = context.filesDir
            val identityFile = File(storageDir, IDENTITY_FILE_NAME)

            if (identityFile.exists()) {
                try {
                    val jsonStr = identityFile.readText(StandardCharsets.UTF_8)
                    val json = JSONObject(jsonStr)
                    val devId = json.getString("device_id")
                    val name = json.optString("display_name", defaultDisplayName)
                    val rawPub = CryptoUtils.fromHex(json.getString("public_key"))

                    val rawPrivSeed: ByteArray
                    if (json.has("encrypted_private_key")) {
                        val encHex = json.getString("encrypted_private_key")
                        val ivHex = json.getString("iv")
                        val encryptedBytes = CryptoUtils.fromHex(encHex)
                        val iv = CryptoUtils.fromHex(ivHex)
                        rawPrivSeed = decryptWithMasterKey(encryptedBytes, iv, context)
                    } else {
                        rawPrivSeed = CryptoUtils.fromHex(json.getString("private_key"))
                    }

                    val privKey = CryptoUtils.parsePrivateKey(rawPrivSeed)
                    return DeviceIdentityManager(devId, name, "android", rawPub, privKey)
                } catch (t: Throwable) {
                    // If decryption or corrupted file fails, regenerate cleanly
                    identityFile.delete()
                }
            }

            // Generate new Ed25519 identity
            val kp = CryptoUtils.generateKeyPair()
            val rawPub = CryptoUtils.rawPublicKey(kp.public)
            val rawPrivSeed = CryptoUtils.rawPrivateKeySeed(kp.private)
            val devId = CryptoUtils.fingerprint(rawPub)

            val json = JSONObject()
            json.put("device_id", devId)
            json.put("display_name", defaultDisplayName)
            json.put("platform", "android")
            json.put("public_key", CryptoUtils.toHex(rawPub))

            // Attempt hardware-backed master key encryption
            val encResult = encryptWithMasterKey(rawPrivSeed, context)
            if (encResult != null) {
                json.put("encrypted_private_key", CryptoUtils.toHex(encResult.ciphertext))
                json.put("iv", CryptoUtils.toHex(encResult.iv))
            } else {
                json.put("private_key", CryptoUtils.toHex(rawPrivSeed))
            }

            identityFile.writeText(json.toString(2), StandardCharsets.UTF_8)
            return DeviceIdentityManager(devId, defaultDisplayName, "android", rawPub, kp.private)
        }

        private data class EncryptionResult(val ciphertext: ByteArray, val iv: ByteArray)

        private fun getOrCreateMasterKey(context: Context): SecretKey? {
            return try {
                val ks = KeyStore.getInstance(ANDROID_KEYSTORE)
                ks.load(null)
                if (ks.containsAlias(MASTER_KEY_ALIAS)) {
                    val entry = ks.getEntry(MASTER_KEY_ALIAS, null) as? KeyStore.SecretKeyEntry
                    return entry?.secretKey
                }

                val keyGen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
                val spec = KeyGenParameterSpec.Builder(
                    MASTER_KEY_ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
                )
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(256)
                    .build()
                keyGen.init(spec)
                keyGen.generateKey()
            } catch (_: Throwable) {
                null
            }
        }

        private fun encryptWithMasterKey(data: ByteArray, context: Context): EncryptionResult? {
            val masterKey = getOrCreateMasterKey(context) ?: return null
            return try {
                val cipher = Cipher.getInstance("AES/GCM/NoPadding")
                cipher.init(Cipher.ENCRYPT_MODE, masterKey)
                val iv = cipher.iv
                val ciphertext = cipher.doFinal(data)
                EncryptionResult(ciphertext, iv)
            } catch (_: Throwable) {
                null
            }
        }

        private fun decryptWithMasterKey(ciphertext: ByteArray, iv: ByteArray, context: Context): ByteArray {
            val masterKey = getOrCreateMasterKey(context)
                ?: throw IllegalStateException("Master key not available for decryption")
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            val spec = GCMParameterSpec(GCM_TAG_LENGTH, iv)
            cipher.init(Cipher.DECRYPT_MODE, masterKey, spec)
            return cipher.doFinal(ciphertext)
        }
    }
}
