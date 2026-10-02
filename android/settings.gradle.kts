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

rootProject.name = "agently-android"

val useSiblingSources = providers
    .gradleProperty("agently.android.useSiblingSources")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_USE_SIBLING_SOURCES"))
    .orElse("false")
    .get()
    .toBooleanStrictOrNull() == true

val forgeSdkDirectory = if (useSiblingSources) {
    file("../../forge/android/sdk")
} else {
    file("deps/forge/android/sdk")
}
val agentlyCoreSdkDirectory = if (useSiblingSources) {
    file("../../agently-core/sdk/android")
} else {
    file("deps/agently-core/sdk/android")
}

check(forgeSdkDirectory.resolve("build.gradle.kts").isFile) {
    if (useSiblingSources) {
        "Forge Android SDK was not found at $forgeSdkDirectory. Check the sibling repository layout."
    } else {
        "Forge Android SDK source is missing. Run: git submodule update --init --recursive"
    }
}
check(agentlyCoreSdkDirectory.resolve("build.gradle.kts").isFile) {
    if (useSiblingSources) {
        "Agently Core Android SDK was not found at $agentlyCoreSdkDirectory. Check the sibling repository layout."
    } else {
        "Agently Core Android SDK source is missing. Run: git submodule update --init --recursive"
    }
}

include(":app")
include(":forge-sdk")
project(":forge-sdk").projectDir = forgeSdkDirectory
include(":agently-core-sdk")
project(":agently-core-sdk").projectDir = agentlyCoreSdkDirectory
