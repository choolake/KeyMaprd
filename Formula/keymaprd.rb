class Keymaprd < Formula
  desc "Remap Logitech MX Master mouse buttons to keyboard shortcuts on macOS"
  homepage "https://github.com/choolake/KeyMapr"
  url "https://github.com/choolake/KeyMapr/archive/refs/tags/v0.1.2.tar.gz"
  # Update sha256 after tagging: `curl -sL <url> | shasum -a 256`
  sha256 "4fbedefd79d94d0ca8cfb9d8ead0549eff86de72511aef65c6fbd4699079c30b"
  license "MIT"
  head "https://github.com/choolake/KeyMapr.git", branch: "main"

  depends_on "go" => :build
  depends_on :macos

  def install
    system "go", "build", "-o", bin/"keymaprd", "./cmd/keymaprd/"
  end

  def caveats
    <<~EOS
      To start keymaprd automatically at login:
        keymaprd install

      keymaprd requires Accessibility access to capture mouse buttons.
      Grant it in System Settings → Privacy & Security → Accessibility.

      Config file location:
        ~/.config/keymaprd/config.json

      Copy the example config to get started:
        mkdir -p ~/.config/keymaprd
        cp #{HOMEBREW_PREFIX}/share/keymaprd/config.example.json ~/.config/keymaprd/config.json
    EOS
  end

  test do
    assert_match "keymaprd v", shell_output("#{bin}/keymaprd --version")
  end
end
