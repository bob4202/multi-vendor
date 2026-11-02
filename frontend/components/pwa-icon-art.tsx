// The app icon artwork (home screen, notifications), rendered to PNG by
// next/og's ImageResponse. Plain inline styles: that renderer doesn't run
// Tailwind.
export function PwaIconArt({ size }: { size: number }) {
  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#171717",
        color: "#ffffff",
        fontSize: size * 0.42,
        fontWeight: 700,
        letterSpacing: -size * 0.02,
      }}
    >
      TH
    </div>
  )
}
