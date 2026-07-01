import 'package:flutter/material.dart';

/// Minimalist cotton palette — white/black with a single violet accent (matches the SSO).
class Palette {
  static const accent = Color(0xFF7C3AED);
  static const accent2 = Color(0xFF6D28D9);

  static const bg = Color(0xFFFFFFFF);
  static const surface = Color(0xFFFAFAFA);
  static const surfaceHover = Color(0xFFF4F4F5);

  static const textPrimary = Color(0xFF09090B); // crisp near-black
  static const textSecondary = Color(0xFF52525B);
  static const textTertiary = Color(0xFF8E8E96);

  static const border = Color(0x1A09090B); // ~10% black hairline
  static const borderStrong = Color(0x2E09090B); // ~18% black
  static const danger = Color(0xFFB91C1C);
  static const success = Color(0xFF15803D);

  // Radii (cotton).
  static const rSm = 10.0;
  static const rMd = 13.0;
  static const rLg = 18.0;

  // Immersive dark surface for the call / recorder screens.
  static const callBg = Color(0xFF09090B);

  // Backwards-compatible aliases used across widgets.
  static const glassFill = surface;
  static const glassFillStrong = surfaceHover;
  static const glassBorder = border;
  static const bg0 = bg;
  static const bg1 = surface;

  static const cardShadow = [
    BoxShadow(color: Color(0x0D09090B), blurRadius: 16, offset: Offset(0, 4)),
  ];

  /// Plain background (strict minimalism — no decorative gradient).
  static const backdrop = LinearGradient(colors: [bg, bg]);
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
    ),
    splashFactory: InkRipple.splashFactory,
    dividerColor: Palette.border,
  );
}
