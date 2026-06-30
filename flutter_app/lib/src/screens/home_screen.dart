import 'package:flutter/material.dart';

import '../state/app_state.dart';
import '../state/call_controller.dart';
import '../sunrise/models.dart';
import '../theme/glass_theme.dart';
import '../widgets/contact_tile.dart';
import '../widgets/glass.dart';
import 'call_screen.dart';
import 'conversation_screen.dart';
import 'incoming_call.dart';

/// Responsive shell: a split master-detail on wide screens (desktop/tablet),
/// and a stack with navigation on narrow screens (phones).
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.state});
  final AppState state;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  String _search = '';
  final _searchCtrl = TextEditingController();

  @override
  void dispose() {
    _searchCtrl.dispose();
    super.dispose();
  }

  List<Contact> get _filtered {
    final all = widget.state.contacts;
    if (_search.isEmpty) return all;
    final q = _search.toLowerCase();
    return all.where((c) => c.name.toLowerCase().contains(q)).toList();
  }

  void _open(Contact c) => widget.state.openConversation(c.topic);

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.of(context).size.width >= 760;
    final call = widget.state.call;
    return Scaffold(
      body: Stack(
        children: [
          DecoratedBox(
            decoration: const BoxDecoration(gradient: Palette.backdrop),
            child: SafeArea(
              child: wide ? _wideLayout() : _narrowLayout(),
            ),
          ),
          if (call.active && call.status == CallStatus.incoming)
            IncomingCallView(call: call)
          else if (call.active)
            CallScreen(call: call),
        ],
      ),
    );
  }

  Widget _wideLayout() {
    final selected = widget.state.currentTopic;
    return Row(
      children: [
        SizedBox(width: 340, child: _sidebar()),
        const VerticalDivider(width: 1, color: Palette.glassBorder),
        Expanded(
          child: selected == null
              ? _welcome()
              : ConversationScreen(state: widget.state, title: _titleFor(selected)),
        ),
      ],
    );
  }

  Widget _narrowLayout() {
    final selected = widget.state.currentTopic;
    if (selected == null) return _sidebar();
    return ConversationScreen(
      state: widget.state,
      title: _titleFor(selected),
      onBack: () => widget.state.openConversationClear(),
    );
  }

  String _titleFor(String topic) {
    final c = widget.state.contacts.firstWhere((c) => c.topic == topic,
        orElse: () => Contact(topic: topic, name: topic));
    return c.name;
  }

  Widget _sidebar() {
    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(10),
          child: GlassPanel(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
            radius: 16,
            child: Row(
              children: [
                GlassAvatar(name: widget.state.displayName, size: 36),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(widget.state.displayName,
                      style: const TextStyle(
                          color: Palette.textPrimary, fontWeight: FontWeight.w600, fontSize: 14)),
                ),
                IconButton(
                    onPressed: widget.state.logout,
                    icon: const Icon(Icons.logout, color: Palette.textSecondary, size: 20)),
              ],
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 10),
          child: GlassField(
            controller: _searchCtrl,
            hint: 'Search…',
            icon: Icons.search,
            onChanged: (v) => setState(() => _search = v),
          ),
        ),
        Expanded(
          child: _filtered.isEmpty
              ? const Center(
                  child: Text('No conversations yet', style: TextStyle(color: Palette.textSecondary)))
              : ListView.builder(
                  padding: const EdgeInsets.all(6),
                  itemCount: _filtered.length,
                  itemBuilder: (ctx, i) {
                    final c = _filtered[i];
                    return ContactTile(
                      contact: c,
                      active: c.topic == widget.state.currentTopic,
                      onTap: () => _open(c),
                    );
                  },
                ),
        ),
      ],
    );
  }

  Widget _welcome() => Center(
        child: GlassPanel(
          padding: const EdgeInsets.all(40),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: const [
              Text('☀️', style: TextStyle(fontSize: 44)),
              SizedBox(height: 12),
              Text('Welcome to Sunrise',
                  style: TextStyle(fontSize: 20, fontWeight: FontWeight.w600, color: Palette.textPrimary)),
              SizedBox(height: 6),
              Text('Select a conversation to start messaging',
                  style: TextStyle(color: Palette.textSecondary, fontSize: 13)),
            ],
          ),
        ),
      );
}
