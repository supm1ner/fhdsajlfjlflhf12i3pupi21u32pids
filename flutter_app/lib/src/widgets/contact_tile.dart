import 'package:flutter/material.dart';

import '../sunrise/models.dart';
import '../theme/glass_theme.dart';
import 'glass.dart';

/// A single conversation row in the sidebar.
class ContactTile extends StatelessWidget {
  const ContactTile({super.key, required this.contact, required this.active, required this.onTap});

  final Contact contact;
  final bool active;
  final VoidCallback onTap;

  /// Compact relative time for the conversation row: "12:30", "Mon", or "3/5".
  static String _relativeTime(DateTime t) {
    final now = DateTime.now();
    final today = DateTime(now.year, now.month, now.day);
    final that = DateTime(t.year, t.month, t.day);
    final days = today.difference(that).inDays;
    if (days <= 0) {
      final h = t.hour.toString().padLeft(2, '0');
      final m = t.minute.toString().padLeft(2, '0');
      return '$h:$m';
    }
    if (days < 7) {
      const names = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
      return names[(t.weekday - 1) % 7];
    }
    return '${t.day}/${t.month}';
  }

  @override
  Widget build(BuildContext context) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(14),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: active ? const Color(0x268B5CF6) : Colors.transparent,
            borderRadius: BorderRadius.circular(14),
          ),
          child: Row(
            children: [
              GlassAvatar(name: contact.name, online: contact.online),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(contact.name,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                            color: Palette.textPrimary, fontWeight: FontWeight.w600, fontSize: 14)),
                    const SizedBox(height: 2),
                    Text(contact.lastMessage.isEmpty ? 'No messages yet' : contact.lastMessage,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(color: Palette.textSecondary, fontSize: 13)),
                  ],
                ),
              ),
              Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  if (contact.touched != null)
                    Text(_relativeTime(contact.touched!),
                        style: TextStyle(
                            color: contact.unread > 0 ? Palette.accent : Palette.textTertiary,
                            fontSize: 11)),
                  if (contact.unread > 0) ...[
                    const SizedBox(height: 4),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                      constraints: const BoxConstraints(minWidth: 18),
                      decoration: BoxDecoration(color: Palette.accent, borderRadius: BorderRadius.circular(9)),
                      child: Text(contact.unread > 99 ? '99+' : '${contact.unread}',
                          textAlign: TextAlign.center,
                          style: const TextStyle(color: Colors.white, fontSize: 11, fontWeight: FontWeight.w600)),
                    ),
                  ],
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
