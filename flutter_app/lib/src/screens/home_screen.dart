import 'dart:async';
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
import 'livekit_room_screen.dart';

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
  bool _searchMode = false;
  Timer? _debounce;

  @override
  void dispose() {
    _debounce?.cancel();
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

  void _onSearchChanged(String v) {
    _debounce?.cancel();
    if (v.length < 2) {
      _searchMode = false;
      _search = v;
      widget.state.clearSearch();
      return;
    }
    _debounce = Timer(const Duration(milliseconds: 300), () {
      setState(() => _search = v);
      _searchMode = true;
      widget.state.searchUsers(v);
    });
  }

  void _openSearchResult(Map<String, dynamic> r) {
    final topic = r['topic'] as String;
    final name = r['name'] as String?;
    widget.state.openConversationWith(topic, name: name);
    setState(() {
      _searchMode = false;
      _search = '';
      _searchCtrl.clear();
    });
    widget.state.clearSearch();
  }

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
            CallScreen(call: call)
          else if (widget.state.liveKit.active)
            LiveKitRoomScreen(controller: widget.state.liveKit),
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
            hint: 'Search users…',
            icon: Icons.search,
            onChanged: _onSearchChanged,
          ),
        ),
        Expanded(
          child: _searchMode
              ? _searchResults()
              : _filtered.isEmpty
                  ? const Center(
                      child: Text('No conversations yet',
                          style: TextStyle(color: Palette.textSecondary)))
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

  Widget _searchResults() {
    final results = widget.state.searchResults;
    final busy = widget.state.searchBusy;
    if (busy) {
      return const Center(child: CircularProgressIndicator(color: Palette.accent));
    }
    if (results.isEmpty) {
      return const Center(
        child: Text('No users found', style: TextStyle(color: Palette.textSecondary)),
      );
    }
    return ListView.builder(
      padding: const EdgeInsets.all(6),
      itemCount: results.length,
      itemBuilder: (ctx, i) {
        final r = results[i];
        return ListTile(
          leading: GlassAvatar(name: r['name'] as String, size: 36),
          title: Text(r['name'] as String,
              style: const TextStyle(color: Palette.textPrimary, fontSize: 14)),
          subtitle: Text(r['topic'] as String,
              style: const TextStyle(color: Palette.textTertiary, fontSize: 11)),
          onTap: () => _openSearchResult(r),
        );
      },
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
