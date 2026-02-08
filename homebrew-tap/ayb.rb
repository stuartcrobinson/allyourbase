class Ayb < Formula
  desc "Backend-as-a-Service for PostgreSQL. Single binary, one config file."
  homepage "https://github.com/stuartcrobinson/allyourbase"
  version "0.1.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/stuartcrobinson/allyourbase/releases/download/v#{version}/ayb_#{version}_darwin_arm64.tar.gz"
      sha256 "cfd93e6bbe95b6c8dd452fff9cf88fa50d842e4b987e7a0a654cbaa515fef6d4"
    end
    on_intel do
      url "https://github.com/stuartcrobinson/allyourbase/releases/download/v#{version}/ayb_#{version}_darwin_amd64.tar.gz"
      sha256 "f94058452c768a4ee206df9fc3aecc1c010df53d6c1501bc922f0ffa3b58a27e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/stuartcrobinson/allyourbase/releases/download/v#{version}/ayb_#{version}_linux_arm64.tar.gz"
      sha256 "3d3d18d005ba352f8a94fcaf4e7da492684e67f24a24a5d5d4190887ed55ca57"
    end
    on_intel do
      url "https://github.com/stuartcrobinson/allyourbase/releases/download/v#{version}/ayb_#{version}_linux_amd64.tar.gz"
      sha256 "7b304b28b3de87931efb708948f9047667f8a28fbb528cd513c6c53ec2ded041"
    end
  end

  def install
    bin.install "ayb"
  end

  test do
    assert_match "ayb", shell_output("#{bin}/ayb version")
  end
end
