// Spike 05 — isolated Gradle root. Self-contained on purpose:
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
rootProject.name = "phonebridge-spike05"
include(":app")
// A second, *separate* application id used as an independent clipboard
// writer/reader peer. It exists so the spike can distinguish "clipboard has
// content" from "our own process wrote it", and so cross-app focus
// transitions can be driven without leaving the spike's own code.
include(":setter")
// The minimal companion IME. Also a separate application id: an IME must be
// installed and *selected* by the user, so it has to be an independent package.
// It exists only to measure the input-method clipboard exemption; it is not a
// product IME.
include(":ime")
