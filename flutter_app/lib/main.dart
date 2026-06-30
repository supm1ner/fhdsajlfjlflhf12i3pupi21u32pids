import 'package:flutter/material.dart';

import 'src/screens/home_screen.dart';
import 'src/screens/login_screen.dart';
import 'src/state/app_state.dart';
import 'src/theme/glass_theme.dart';

void main() {
  runApp(const SunriseApp());
}

class SunriseApp extends StatefulWidget {
  const SunriseApp({super.key});

  @override
  State<SunriseApp> createState() => _SunriseAppState();
}

class _SunriseAppState extends State<SunriseApp> {
  final AppState _state = AppState();

  @override
  void dispose() {
    _state.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Sunrise',
      debugShowCheckedModeBanner: false,
      theme: buildGlassTheme(),
      home: ListenableBuilder(
        listenable: Listenable.merge([_state, _state.call, _state.liveKit]),
        builder: (context, _) {
          return _state.phase == Phase.ready
              ? HomeScreen(state: _state)
              : LoginScreen(state: _state);
        },
      ),
    );
  }
}
