import 'package:flutter/material.dart';

/// Minimalist light palette — white/black with a violet accent.
class Palette {
  static const accent = Color(0xFF7C3AED);
  static const accent2 = Color(0xFF6D28D9);

  static const bg = Color(0xFFFFFFFF);
  static const surface = Color(0xFFF7F7F8);
  static const surfaceHover = Color(0x0D000000); // 5% black

  static const textPrimary = Color(0xFF0A0A0A);
  static const textSecondary = Color(0x8F0A0A0A); // 56%
  static const textTertiary = Color(0x610A0A0A); // 38%

  static const border = Color(0x14000000); // 8% black
  static const danger = Color(0xFFEF4444);
  static const success = Color(0xFF22C55E);

  // Immersive dark surface for the call screen (stays dark like Telegram).
  static const callBg = Color(0xFF0B0B12);

  // Backwards-compatible aliases used across widgets.
  static const glassFill = Color(0x08000000); // 3% black
  static const glassFillStrong = Color(0x14000000); // 8% black
  static const glassBorder = border;
  static const bg0 = bg;
  static const bg1 = surface;

  static const cardShadow = [
    BoxShadow(color: Color(0x12000000), blurRadius: 24, offset: Offset(0, 4)),
  ];

  /// Near-white backdrop with a faint violet glow in the corner.
  static const backdrop = RadialGradient(
    center: Alignment(0.9, -0.9),
    radius: 1.3,
    colors: [Color(0x147C3AED), bg],
    stops: [0.0, 0.6],
  );
}

ThemeData buildGlassTheme() {
  final base = ThemeData.light(useMaterial3: true);
  return base.copyWith(
    scaffoldBackgroundColor: Palette.bg,
    colorScheme: base.colorScheme.copyWith(
      primary: Palette.accent,
      secondary: Palette.accent,
      surface: Palette.bg,
      error: Palette.danger,
      brightness: Brightness.light,
    ),
    textTheme: base.textTheme.apply(
      bodyColor: Palette.textPrimary,
      displayColor: Palette.textPrimary,
      fontFamily: 'Inter',
    ),
    splashFactory: InkRipple.splashFactory,
  );
}
