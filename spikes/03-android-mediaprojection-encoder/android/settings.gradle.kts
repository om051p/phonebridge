// Spike 03 — isolated Gradle root. Self-contained on purpose:
// this project must NOT be wired into android/ (production) in any way.
pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}
dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}
rootProject.name = "phonebridge-spike03"
include(":app")
