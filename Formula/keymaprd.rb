class Keymaprd < Formula
  desc "Remap Logitech MX Master mouse buttons to keyboard shortcuts on macOS"
  homepage "https://github.com/choolake/keymaprd"
  url "https://github.com/choolake/keymaprd/archive/refs/tags/v0.1.0.tar.gz"
  # Update sha256 after tagging: `brew fetch --build-from-source Formula/keymaprd.rb`
  sha256 "PLACEHOLDER_UPDATE_AFTER_FIRST_RELEASE_TAG"
  license "MIT"
  head "https://github.com/choolake/keymaprd.git", branch: "main"

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
