#
# To learn more about a Podspec see http://guides.cocoapods.org/syntax/podspec.html.
# Run `pod lib lint flutter_openim_sdk.podspec` to validate before publishing.
#
Pod::Spec.new do |s|
  s.name             = 'flutter_openim_sdk'
  s.version          = '0.0.1'
  s.summary          = 'A new Flutter project.'
  s.description      = <<-DESC
A new Flutter project.
                       DESC
  s.homepage         = 'http://example.com'
  s.license          = { :file => '../LICENSE' }
  s.author           = { 'Your Company' => 'email@example.com' }
  s.source           = { :path => '.' }
  s.source_files = 'Classes/**/*'
  s.public_header_files = 'Classes/FlutterOpenimSdkPlugin.h', 'Classes/PMFragmentMuxer.h', 'Classes/PMEncryptedEncoder.h'
  s.dependency 'Flutter'
  s.platform = :ios, '13.0'

  # Local controlled core is mandatory: the old remote framework has plaintext
  # SQLite and must never be a silent fallback. Build this on macOS first.
  s.vendored_frameworks = 'Framework/OpenIMCore.xcframework', 'Framework/SQLCipher.xcframework'
  s.static_framework = true
  s.library = 'resolv'
  s.frameworks = 'AVFoundation', 'AVKit', 'Photos', 'VideoToolbox', 'AudioToolbox', 'CoreMedia', 'CoreVideo', 'CoreImage', 'ImageIO', 'Security', 'CryptoKit', 'MobileCoreServices', 'Network'

  # s.vendored_frameworks = 'Framework/*.xcframework'
  # Flutter.framework does not contain a i386 slice.
  s.pod_target_xcconfig = { 'DEFINES_MODULE' => 'YES', 'EXCLUDED_ARCHS[sdk=iphonesimulator*]' => 'i386' }
  s.swift_version = '5.0'

  s.resource_bundles = {'flutter_openim_sdk_privacy' => ['Resources/PrivacyInfo.xcprivacy', 'Resources/puty_enterprise_root_ca.der']}
end
