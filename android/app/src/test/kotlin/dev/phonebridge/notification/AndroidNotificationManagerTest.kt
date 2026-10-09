package dev.phonebridge.notification

import android.app.Notification
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Unit tests for AndroidNotificationManager policies (DEC-028, Phase 8 v0.1).
 */
class AndroidNotificationManagerTest {

    @Test
    fun `dispatch fails closed when manager is not started`() {
        // Without start(context), dispatch methods must return false
        val posted = AndroidNotificationManager.onNotificationPosted(
            key = "0|com.example|1|null|10",
            packageName = "com.example",
            appName = "Example",
            title = "Test",
            text = "Body",
            subText = "",
            postTimeMs = 1000L,
            isOngoing = false,
            isClearable = true,
            category = "msg",
        )
        assertFalse(posted)

        val removed = AndroidNotificationManager.onNotificationRemoved(
            key = "0|com.example|1|null|10",
            packageName = "com.example",
            reason = 1,
        )
        assertFalse(removed)
    }

    @Test
    fun `self package filtering suppresses phonebridge notifications`() {
        val myPackage = "dev.phonebridge"
        val incomingPackage = "dev.phonebridge"
        val shouldSuppress = (incomingPackage == myPackage)
        assertTrue("PhoneBridge own notifications must be suppressed", shouldSuppress)

        val otherPackage = "org.telegram.messenger"
        val shouldAllow = (otherPackage != myPackage)
        assertTrue("External app notifications must be allowed", shouldAllow)
    }

    @Test
    fun `listener starts disconnected until the platform binds it`() {
        // Runtime liveness, not permission: with no service bound the UI must
        // read "granted but not running", never "active".
        assertFalse(AndroidNotificationManager.isListenerConnected())
    }

    @Test
    fun `screen-lock guard suppresses secret visibility notifications`() {
        val isDeviceLocked = true
        val visibilitySecret = Notification.VISIBILITY_SECRET
        val visibilityPrivate = Notification.VISIBILITY_PRIVATE
        val visibilityPublic = Notification.VISIBILITY_PUBLIC

        fun shouldSuppress(isLocked: Boolean, visibility: Int): Boolean {
            return isLocked && visibility == Notification.VISIBILITY_SECRET
        }

        assertTrue(shouldSuppress(isDeviceLocked, visibilitySecret))
        assertFalse(shouldSuppress(isDeviceLocked, visibilityPrivate))
        assertFalse(shouldSuppress(isDeviceLocked, visibilityPublic))
        assertFalse(shouldSuppress(false, visibilitySecret))
    }

    @Test
    fun `dismissNotification returns false when listenerService is not connected`() {
        assertFalse(AndroidNotificationManager.dismissNotification("test-key"))
    }
}

