import 'package:flutter/material.dart';

/// Liquid-glass palette and theme. Dark, minimalist, with a violet accent matching
/// the web client.
class Palette {
  static const accent = Color(0xFF8B5CF6);
  static const accent2 = Color(0xFF6D28D9);
  static const bg0 = Color(0xFF0B0B12);
  static const bg1 = Color(0xFF14141C);
  static const textPrimary = Color(0xFFF5F5FA);
  static const textSecondary = Color(0xB3F5F5FA);
  static const textTertiary = Color(0x80F5F5FA);
  static const danger = Color(0xFFEF4444);
  static const success = Color(0xFF22C55E);

  static const glassFill = Color(0x14FFFFFF); // ~8% white
  static const glassFillStrong = Color(0x26FFFFFF);
  static const glassBorder = Color(0x1FFFFFFF);

  /// Ambient background gradient behind the frosted glass.
  static const backdrop = RadialGradient(
    center: Alignment(0.8, -0.9),
    radius: 1.4,
    colors: [Color(0xFF241A4D), bg0],
    stops: [0.0, 0.7],
  );
}

ThemeData buildGlassTheme() {
  final base = ThemeData.dark(useMaterial3: true);
  return base.copyWith(
    scaffoldBackgroundColor: Palette.bg0,
    colorScheme: base.colorScheme.copyWith(
      primary: Palette.accent,
      secondary: Palette.accent,
      surface: Palette.bg1,
      error: Palette.danger,
    ),
    textTheme: base.textTheme.apply(
      bodyColor: Palette.textPrimary,
      displayColor: Palette.textPrimary,
      fontFamily: 'Inter',
    ),
    splashFactory: InkRipple.splashFactory,
  );
}
