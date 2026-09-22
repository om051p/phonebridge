allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

// Standard Flutter template wiring: keep Gradle outputs under the Flutter
// project's build/ directory so flutter build/run can locate the APKs.
// NOTE: ui/android is a symlink to ../android, so Gradle canonicalizes the
// root project to <repo>/android. From its build dir, ../../ui/build is the
// Flutter project build/ directory the flutter tool expects.
val newBuildDir: Directory = rootProject.layout.buildDirectory.dir("../../ui/build").get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
