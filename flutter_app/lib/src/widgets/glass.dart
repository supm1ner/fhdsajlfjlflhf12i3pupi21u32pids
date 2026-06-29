import 'dart:ui';

import 'package:flutter/material.dart';

import '../theme/glass_theme.dart';

/// A frosted-glass surface: blurred backdrop, translucent fill, hairline border.
class GlassPanel extends StatelessWidget {
  const GlassPanel({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(16),
    this.radius = 20,
    this.blur = 18,
    this.strong = false,
  });

  final Widget child;
  final EdgeInsets padding;
  final double radius;
  final double blur;
  final bool strong;

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(radius),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: blur, sigmaY: blur),
        child: Container(
          padding: padding,
          decoration: BoxDecoration(
            gradient: LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: strong
                  ? const [Color(0x33FFFFFF), Color(0x14FFFFFF)]
                  : const [Color(0x1AFFFFFF), Color(0x0DFFFFFF)],
            ),
            borderRadius: BorderRadius.circular(radius),
            border: Border.all(color: Palette.glassBorder, width: 1),
          ),
          child: child,
        ),
      ),
    );
  }
}

/// A glass pill button.
class GlassButton extends StatelessWidget {
  const GlassButton({
    super.key,
    required this.label,
    this.onTap,
    this.icon,
    this.primary = false,
    this.expand = false,
    this.loading = false,
  });

  final String label;
  final VoidCallback? onTap;
  final IconData? icon;
  final bool primary;
  final bool expand;
  final bool loading;

  @override
  Widget build(BuildContext context) {
    final content = Row(
      mainAxisSize: expand ? MainAxisSize.max : MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        if (loading)
          const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
        else ...[
          if (icon != null) ...[Icon(icon, size: 18), const SizedBox(width: 8)],
          Text(label, style: const TextStyle(fontWeight: FontWeight.w600, fontSize: 14)),
        ],
      ],
    );
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: loading ? null : onTap,
        borderRadius: BorderRadius.circular(14),
        child: Ink(
          decoration: BoxDecoration(
            gradient: primary
                ? const LinearGradient(colors: [Palette.accent, Palette.accent2])
                : null,
            color: primary ? null : Palette.glassFill,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: primary ? Colors.transparent : Palette.glassBorder),
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 13),
            child: content,
          ),
        ),
      ),
    );
  }
}

/// A glass text field.
class GlassField extends StatelessWidget {
  const GlassField({
    super.key,
    required this.controller,
    this.hint = '',
    this.obscure = false,
    this.icon,
    this.onSubmitted,
    this.onChanged,
  });

  final TextEditingController controller;
  final String hint;
  final bool obscure;
  final IconData? icon;
  final ValueChanged<String>? onSubmitted;
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: Palette.glassFill,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: Palette.glassBorder),
      ),
      child: TextField(
        controller: controller,
        obscureText: obscure,
        onSubmitted: onSubmitted,
        onChanged: onChanged,
        style: const TextStyle(color: Palette.textPrimary, fontSize: 14),
        decoration: InputDecoration(
          hintText: hint,
          hintStyle: const TextStyle(color: Palette.textTertiary),
          prefixIcon: icon != null ? Icon(icon, size: 18, color: Palette.textSecondary) : null,
          border: InputBorder.none,
          contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        ),
      ),
    );
  }
}

/// Circular avatar with initials fallback.
class GlassAvatar extends StatelessWidget {
  const GlassAvatar({super.key, required this.name, this.size = 44, this.online = false});
  final String name;
  final double size;
  final bool online;

  String get _initials {
    final parts = name.trim().split(RegExp(r'\s+'));
    final letters = parts.where((p) => p.isNotEmpty).map((p) => p[0]).take(2).join();
    return letters.isEmpty ? '?' : letters.toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: Stack(
        children: [
          Container(
            width: size,
            height: size,
            decoration: const BoxDecoration(
              shape: BoxShape.circle,
              gradient: LinearGradient(colors: [Palette.accent, Palette.accent2]),
            ),
            alignment: Alignment.center,
            child: Text(_initials,
                style: TextStyle(color: Colors.white, fontWeight: FontWeight.w600, fontSize: size * 0.36)),
          ),
          if (online)
            Positioned(
              right: 0,
              bottom: 0,
              child: Container(
                width: size * 0.28,
                height: size * 0.28,
                decoration: BoxDecoration(
                  color: Palette.success,
                  shape: BoxShape.circle,
                  border: Border.all(color: Palette.bg0, width: 2),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
