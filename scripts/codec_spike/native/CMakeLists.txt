cmake_minimum_required(VERSION 3.24)
project(PulsarMediaFoundationProbe LANGUAGES CXX)

add_executable(PulsarMediaFoundationProbe WIN32 mf_appcontainer_probe.cpp)
target_compile_features(PulsarMediaFoundationProbe PRIVATE cxx_std_20)
target_compile_definitions(PulsarMediaFoundationProbe PRIVATE UNICODE _UNICODE WIN32_LEAN_AND_MEAN)
target_compile_options(PulsarMediaFoundationProbe PRIVATE /W4 /WX /permissive- /EHsc)
target_link_options(PulsarMediaFoundationProbe PRIVATE /APPCONTAINER /DYNAMICBASE /NXCOMPAT)
target_link_libraries(PulsarMediaFoundationProbe PRIVATE
  mfplat
  mfreadwrite
  mfuuid
  ole32
  psapi
)
set_property(TARGET PulsarMediaFoundationProbe PROPERTY
  MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")
