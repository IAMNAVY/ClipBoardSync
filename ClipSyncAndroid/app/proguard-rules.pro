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
