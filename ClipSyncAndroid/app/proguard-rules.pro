# Keep OkHttp
-dontwarn okhttp3.**
-keep class okhttp3.** { *; }

# Keep kotlinx.serialization
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.AnnotationsKt
-keepclassmembers class kotlinx.serialization.json.** { *** Companion; }
-keepclasseswithmembers class kotlinx.serialization.json.** {
    kotlinx.serialization.KSerializer serializer(...);
}
-keep,includedescriptorclasses class com.clipsync.android.**$$serializer { *; }
-keepclassmembers class com.clipsync.android.** {
    *** Companion;
}
-keepclasseswithmembers class com.clipsync.android.** {
    kotlinx.serialization.KSerializer serializer(...);
}

# Keep clipboard monitoring classes - critical for release mode
# These classes use system-level APIs (logcat, floating windows)
# that R8 might mistake as dead code
-keep class com.clipsync.android.clipboard.** { *; }
-keep class com.clipsync.android.service.** { *; }

# Keep AppConfig data classes (used with DataStore serialization)
-keep class com.clipsync.android.data.** { *; }

# Keep network classes (OkHttp callbacks can be stripped)
-keep class com.clipsync.android.network.** { *; }
