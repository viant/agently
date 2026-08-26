plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

fun gitOutput(vararg arguments: String): String {
    return runCatching {
        val command = listOf(
            "git",
            "-C",
            rootProject.projectDir.parentFile.absolutePath,
        ) + arguments
        val process = ProcessBuilder(command)
            .redirectErrorStream(true)
            .start()
        val output = process.inputStream.bufferedReader().use { it.readText() }.trim()
        check(process.waitFor() == 0) { output }
        output
    }.getOrDefault("")
}

val agentlyGitRevision = providers
    .gradleProperty("agently.gitRevision")
    .orElse(providers.environmentVariable("AGENTLY_GIT_REVISION"))
    .orElse(gitOutput("rev-parse", "HEAD"))
    .get()
    .ifBlank { "unknown" }
val agentlyGitVersion = providers
    .gradleProperty("agently.gitVersion")
    .orElse(providers.environmentVariable("AGENTLY_GIT_VERSION"))
    .orElse(gitOutput("describe", "--tags", "--always", "--dirty"))
    .get()
    .ifBlank { agentlyGitRevision.take(12) }
val agentlyGitVersionCode = providers
    .gradleProperty("agently.gitVersionCode")
    .orElse(providers.environmentVariable("AGENTLY_GIT_VERSION_CODE"))
    .orElse(gitOutput("rev-list", "--count", "HEAD"))
    .get()
    .toIntOrNull()
    ?.coerceAtLeast(1)
    ?: 1

val agentlyAndroidBaseUrlProvider = providers
    .gradleProperty("agently.android.baseUrl")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_BASE_URL"))
val agentlyAndroidBaseUrl = agentlyAndroidBaseUrlProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")
val agentlyAndroidBaseUrlExplicit = agentlyAndroidBaseUrlProvider.isPresent
val agentlyAndroidOauthConfigProvider = providers
    .gradleProperty("agently.android.oauthConfigUrl")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_OAUTH_CONFIG_URL"))
val agentlyAndroidOauthConfig = agentlyAndroidOauthConfigProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")
val agentlyAndroidOobSecretProvider = providers
    .gradleProperty("agently.android.oobSecretRef")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_OOB_SECRET_REF"))
val agentlyAndroidOobSecret = agentlyAndroidOobSecretProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")
val agentlyAndroidAutoOobProvider = providers
    .gradleProperty("agently.android.autoOobSignIn")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_AUTO_OOB_SIGN_IN"))
val agentlyAndroidAutoOob = if (agentlyAndroidAutoOobProvider.orElse("false").get().trim().equals("true", ignoreCase = true)) {
    "true"
} else {
    "false"
}
val agentlyAndroidIdpUserProvider = providers
    .gradleProperty("agently.android.idpUsername")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_IDP_USERNAME"))
val agentlyAndroidIdpUser = agentlyAndroidIdpUserProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")
val agentlyAndroidIdpPasswordProvider = providers
    .gradleProperty("agently.android.idpPassword")
    .orElse(providers.environmentVariable("AGENTLY_ANDROID_IDP_PASSWORD"))
val agentlyAndroidIdpPassword = agentlyAndroidIdpPasswordProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")
val agentlyAndroidWorkspaceEndpointsProvider = providers
    .gradleProperty("agently.android.workspaceEndpointsJson")
    .orElse(providers.environmentVariable("AGENTLY_WORKSPACE_ENDPOINTS_JSON"))
val agentlyAndroidWorkspaceEndpoints = agentlyAndroidWorkspaceEndpointsProvider
    .orElse("")
    .get()
    .replace("\\", "\\\\")
    .replace("\"", "\\\"")

android {
    namespace = "com.viant.agently.android"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.viant.agently.android"
        minSdk = 26
        targetSdk = 35
        versionCode = agentlyGitVersionCode
        versionName = agentlyGitVersion
        buildConfigField("String", "GIT_REVISION", "\"$agentlyGitRevision\"")
        buildConfigField("String", "GIT_VERSION", "\"$agentlyGitVersion\"")
        buildConfigField("String", "APP_API_BASE_URL", "\"$agentlyAndroidBaseUrl\"")
        buildConfigField("boolean", "APP_API_BASE_URL_EXPLICIT", agentlyAndroidBaseUrlExplicit.toString())
        buildConfigField("String", "BOOTSTRAP_OAUTH_CONFIG_URL", "\"$agentlyAndroidOauthConfig\"")
        buildConfigField("String", "BOOTSTRAP_OOB_SECRET_REF", "\"$agentlyAndroidOobSecret\"")
        buildConfigField("boolean", "BOOTSTRAP_AUTO_OOB_SIGN_IN", agentlyAndroidAutoOob)
        buildConfigField("String", "BOOTSTRAP_IDP_USERNAME", "\"$agentlyAndroidIdpUser\"")
        buildConfigField("String", "BOOTSTRAP_IDP_PASSWORD", "\"$agentlyAndroidIdpPassword\"")
        buildConfigField("String", "WORKSPACE_ENDPOINTS_JSON", "\"$agentlyAndroidWorkspaceEndpoints\"")
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    composeOptions {
        kotlinCompilerExtensionVersion = "1.5.14"
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    packaging {
        jniLibs {
            useLegacyPackaging = true
        }
    }

}

dependencies {
    implementation(project(":forge-sdk"))
    implementation(project(":agently-core-sdk"))

    val composeBom = platform("androidx.compose:compose-bom:2024.09.01")
    implementation(composeBom)

    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.4")
    implementation("androidx.activity:activity-compose:1.9.2")
    implementation("androidx.security:security-crypto:1.1.0-alpha06")

    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-tooling-preview")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.6.3")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
}
