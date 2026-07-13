cmake_minimum_required(VERSION 3.24)

project(pulsar_capture LANGUAGES CXX)

if(NOT MSVC)
  message(FATAL_ERROR "pulsar-capture requires MSVC and the Windows 10 SDK")
endif()

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_MSVC_RUNTIME_LIBRARY "MultiThreaded$<$<CONFIG:Debug>:Debug>")

set(PULSAR_CAPTURE_LIBS
  windowsapp
  runtimeobject
  ole32
  mmdevapi
)

add_library(pulsar-capture SHARED pulsar_capture.cpp)
target_compile_definitions(pulsar-capture PRIVATE
  PULSAR_CAPTURE_EXPORTS
  UNICODE
  _UNICODE
  WIN32_LEAN_AND_MEAN
  NOMINMAX
)
target_compile_options(pulsar-capture PRIVATE /W4 /permissive- /EHsc /utf-8)
target_link_libraries(pulsar-capture PRIVATE ${PULSAR_CAPTURE_LIBS})
set_target_properties(pulsar-capture PROPERTIES
  OUTPUT_NAME "pulsar-capture"
)

include(CTest)
if(BUILD_TESTING)
  add_executable(pulsar-capture-tests
    tests/pulsar_capture_tests.cpp
    pulsar_capture.cpp
  )
  target_compile_definitions(pulsar-capture-tests PRIVATE
    PULSAR_CAPTURE_STATIC
    UNICODE
    _UNICODE
    WIN32_LEAN_AND_MEAN
    NOMINMAX
  )
  target_compile_options(pulsar-capture-tests PRIVATE /W4 /permissive- /EHsc /utf-8)
  target_link_libraries(pulsar-capture-tests PRIVATE ${PULSAR_CAPTURE_LIBS})
  add_test(NAME pulsar-capture-tests COMMAND pulsar-capture-tests)
endif()
