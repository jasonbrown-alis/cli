gRPC C++ - Building from source
===========================

The following is taken from: [gRPC C++ - Building from source](https://github.com/grpc/grpc/blob/master/BUILDING.md#clone-the-repository-including-submodules)

This document has detailed instructions on how to build gRPC C++ from source. Note that it only covers the build of gRPC itself and is mostly meant for gRPC C++ contributors and/or power users.
Other should follow the user instructions. See the [How to use](https://github.com/grpc/grpc/tree/master/src/cpp#to-start-using-grpc-c) instructions for guidance on how to add gRPC as a dependency to a C++ application (there are several ways and system wide installation is often not the best choice).

# Pre-requisites

## Linux

```sh
 $ [sudo] apt-get install build-essential autoconf libtool pkg-config
```

If you plan to build using CMake
```sh
 $ [sudo] apt-get install cmake
```

If you are a contributor and plan to build and run tests, install the following as well:
```sh
 $ # clang and LLVM C++ lib is only required for sanitizer builds
 $ [sudo] apt-get install clang libc++-dev
```

## MacOS

On a Mac, you will first need to
install Xcode or
[Command Line Tools for Xcode](https://developer.apple.com/download/more/)
and then run the following command from a terminal:

```sh
 $ [sudo] xcode-select --install
```

To build gRPC from source, you may need to install the following
packages from [Homebrew](https://brew.sh):

```sh
 $ brew install autoconf automake libtool shtool
```

If you plan to build using CMake, follow the instructions from https://cmake.org/download/

*Tip*: when building,
you *may* want to explicitly set the `LIBTOOL` and `LIBTOOLIZE`
environment variables when running `make` to ensure the version
installed by `brew` is being used:

```sh
 $ LIBTOOL=glibtool LIBTOOLIZE=glibtoolize make
```

## Windows

To prepare for cmake + Microsoft Visual C++ compiler build
- Install Visual Studio 2015 or 2017 (Visual C++ compiler will be used).
- Install [Git](https://git-scm.com/).
- Install [CMake](https://cmake.org/download/).
- Install [nasm](https://www.nasm.us/) and add it to `PATH` (`choco install nasm`) - *required by boringssl*
- (Optional) Install [Ninja](https://ninja-build.org/) (`choco install ninja`)

#  Cloning the repository
Before building, you need to clone the gRPC github repository and download submodules containing source code for gRPC's dependencies (that's done by the submodule command or --recursive flag). Use following commands to clone the gRPC repository at the latest stable release tag


```shell
LATEST_VER=$(curl --silent "https://api.github.com/repos/grpc/grpc/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
git clone -b $LATEST_VER https://github.com/grpc/grpc

cd grpc

git submodule update --init
```

# Build From Source
In the C++ world, there's no "standard" build system that would work for in all supported use cases and on all supported platforms. Therefore, gRPC supports several major build systems, which should satisfy most users. Depending on your needs we recommend building using bazel or cmake.

```shell
export GRPC_INSTALL_DIR=$HOME/.local
mkdir -p $GRPC_INSTALL_DIR
export PATH="$GRPC_INSTALL_DIR/bin:$PATH"

brew install cmake

mkdir -p cmake/build
pushd cmake/build
cmake -DgRPC_INSTALL=ON \
      -DgRPC_BUILD_TESTS=OFF \
      -DCMAKE_INSTALL_PREFIX=$GRPC_INSTALL_DIR \
      ../..
make -j4
sudo make install
popd

mkdir -p third_party/abseil-cpp/cmake/build
pushd third_party/abseil-cpp/cmake/build
cmake -DCMAKE_INSTALL_PREFIX=$GRPC_INSTALL_DIR \
      -DCMAKE_POSITION_INDEPENDENT_CODE=TRUE \
      ../..
make -j4
sudo make install
popd
```