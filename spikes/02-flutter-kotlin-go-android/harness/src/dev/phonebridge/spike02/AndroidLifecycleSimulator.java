package dev.phonebridge.spike02;

/**
 * Simulates Android Service lifecycle states and validates Go core behavior across transitions.
 */
public class AndroidLifecycleSimulator {

    public enum ServiceState {
        UNCREATED,
        CREATED,
        RUNNING,
        TRIMMED,
        DESTROYED
    }

    // Android ComponentCallbacks2 trim memory levels
    public static final int TRIM_MEMORY_RUNNING_MODERATE = 5;
    public static final int TRIM_MEMORY_RUNNING_LOW = 10;
    public static final int TRIM_MEMORY_RUNNING_CRITICAL = 15;
    public static final int TRIM_MEMORY_UI_HIDDEN = 20;
    public static final int TRIM_MEMORY_BACKGROUND = 40;
    public static final int TRIM_MEMORY_COMPLETE = 80;

    private ServiceState currentState = ServiceState.UNCREATED;
    private final String filesDir;

    public AndroidLifecycleSimulator(String filesDir) {
        this.filesDir = filesDir;
    }

    public ServiceState getState() {
        return currentState;
    }

    /**
     * Simulates Service.onCreate()
     */
    public void onCreate() {
        if (currentState != ServiceState.UNCREATED && currentState != ServiceState.DESTROYED) {
            throw new IllegalStateException("Service already created: " + currentState);
        }
        GoBridge.start(filesDir);
        currentState = ServiceState.CREATED;
    }

    /**
     * Simulates Service.onStartCommand()
     */
    public void onStartCommand() {
        if (currentState != ServiceState.CREATED && currentState != ServiceState.TRIMMED && currentState != ServiceState.RUNNING) {
            throw new IllegalStateException("Service must be created before start: " + currentState);
        }
        currentState = ServiceState.RUNNING;
    }

    /**
     * Simulates ComponentCallbacks2.onTrimMemory()
     */
    public void onTrimMemory(int level) {
        if (currentState == ServiceState.RUNNING || currentState == ServiceState.TRIMMED) {
            GoBridge.trimMemory(level);
            currentState = ServiceState.TRIMMED;
        }
    }

    /**
     * Simulates Service.onDestroy()
     */
    public void onDestroy() {
        GoBridge.shutdown();
        currentState = ServiceState.DESTROYED;
    }
}
