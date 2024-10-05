# panago
Golang code to explore and research panasonic bluray player

Build Instructions

Edit the .goreleaser.yaml for updating build targets

To test on local machine run

goreleaser release --snapshot --skip=publish --clean

This requires goreleaser, to test without use the build.sh script


To trigger an official release

#git tag v0.1.1

#git push origin v0.1.1

This will build and publish all versions of the binary in release section
 
 

