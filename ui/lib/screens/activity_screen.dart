import 'package:flutter/material.dart';
import '../controllers/phonebridge_controller.dart';
import '../models/activity_event.dart';
import '../ui/transfer_views.dart';

class ActivityScreen extends StatefulWidget {
  const ActivityScreen({
    super.key,
    required this.controller,
  });

  final PhoneBridgeController controller;

  @override
  State<ActivityScreen> createState() => _ActivityScreenState();
}

class _ActivityScreenState extends State<ActivityScreen> {
  ActivityCategory? _selectedFilter;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return ListenableBuilder(
      listenable: widget.controller,
      builder: (context, _) {
        final allEvents = widget.controller.activityEvents;
        final filteredEvents = _selectedFilter == null
            ? allEvents
            : allEvents.where((e) => e.category == _selectedFilter).toList();

        // One scroll surface: the transfers section (live progress plus the
        // send-file action) sits above the activity log, so file transfer is not
        // hidden behind a mode the user has to discover first.
        return ListView(
          padding: const EdgeInsets.only(bottom: 16),
          children: [
            const SizedBox(height: 16),
            TransfersSection(controller: widget.controller.transfers),
            const SizedBox(height: 16),
            const Divider(height: 1),
            _buildFilterBar(theme),
            if (filteredEvents.isEmpty)
              _buildEmptyView(theme)
            else
              ..._buildEventRows(theme, filteredEvents),
          ],
        );
      },
    );
  }

  List<Widget> _buildEventRows(ThemeData theme, List<ActivityEvent> events) {
    final rows = <Widget>[];
    for (var i = 0; i < events.length; i++) {
      if (i > 0) rows.add(const Divider(height: 1));
      rows.add(
        Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16),
          child: _buildEventTile(theme, events[i]),
        ),
      );
    }
    return rows;
  }

  Widget _buildFilterBar(ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16.0, vertical: 8.0),
      child: Row(
        children: [
          Expanded(
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: Row(
                children: [
                  FilterChip(
                    label: const Text('All'),
                    selected: _selectedFilter == null,
                    onSelected: (_) => setState(() => _selectedFilter = null),
                    visualDensity: VisualDensity.compact,
                  ),
                  const SizedBox(width: 8),
                  FilterChip(
                    label: const Text('Screen'),
                    selected: _selectedFilter == ActivityCategory.screen,
                    onSelected: (_) => setState(() => _selectedFilter = ActivityCategory.screen),
                    visualDensity: VisualDensity.compact,
                  ),
                  const SizedBox(width: 8),
                  FilterChip(
                    label: const Text('Clipboard'),
                    selected: _selectedFilter == ActivityCategory.clipboard,
                    onSelected: (_) => setState(() => _selectedFilter = ActivityCategory.clipboard),
                    visualDensity: VisualDensity.compact,
                  ),
                  const SizedBox(width: 8),
                  FilterChip(
                    label: const Text('Pairing'),
                    selected: _selectedFilter == ActivityCategory.pairing,
                    onSelected: (_) => setState(() => _selectedFilter = ActivityCategory.pairing),
                    visualDensity: VisualDensity.compact,
                  ),
                  const SizedBox(width: 8),
                  FilterChip(
                    label: const Text('Connection'),
                    selected: _selectedFilter == ActivityCategory.connection,
                    onSelected: (_) => setState(() => _selectedFilter = ActivityCategory.connection),
                    visualDensity: VisualDensity.compact,
                  ),
                ],
              ),
            ),
          ),
          if (widget.controller.activityEvents.isNotEmpty)
            IconButton(
              icon: const Icon(Icons.delete_sweep_outlined, size: 20),
              tooltip: 'Clear Log',
              onPressed: () => widget.controller.clearActivityLog(),
            ),
        ],
      ),
    );
  }

  Widget _buildEventTile(ThemeData theme, ActivityEvent event) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(vertical: 4),
      leading: CircleAvatar(
        radius: 20,
        backgroundColor: _levelColor(event.level, theme).withValues(alpha: 0.15),
        child: Icon(
          _categoryIcon(event.category),
          color: _levelColor(event.level, theme),
          size: 20,
        ),
      ),
      title: Text(
        event.title,
        style: const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
      ),
      subtitle: Text(
        event.detail,
        style: TextStyle(
          color: theme.colorScheme.onSurfaceVariant,
          fontSize: 12,
        ),
      ),
      trailing: Text(
        event.timeAgo,
        style: TextStyle(
          color: theme.colorScheme.onSurfaceVariant,
          fontSize: 11,
        ),
      ),
    );
  }

  Widget _buildEmptyView(ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 48),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(
            Icons.history_toggle_off,
            size: 48,
            color: theme.colorScheme.onSurfaceVariant.withValues(alpha: 0.5),
          ),
          const SizedBox(height: 12),
          Text(
            'No Activity Recorded',
            style: theme.textTheme.titleMedium?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            'Events such as screen sharing, clipboard sync and file transfers '
            'will appear here.',
            textAlign: TextAlign.center,
            style: theme.textTheme.bodySmall?.copyWith(
              color: theme.colorScheme.onSurfaceVariant,
            ),
          ),
        ],
      ),
    );
  }

  IconData _categoryIcon(ActivityCategory category) {
    switch (category) {
      case ActivityCategory.screen:
        return Icons.screen_share;
      case ActivityCategory.clipboard:
        return Icons.content_paste;
      case ActivityCategory.pairing:
        return Icons.link;
      case ActivityCategory.connection:
        return Icons.wifi;
      case ActivityCategory.system:
        return Icons.info_outline;
    }
  }

  Color _levelColor(ActivityLevel level, ThemeData theme) {
    switch (level) {
      case ActivityLevel.success:
        return Colors.green;
      case ActivityLevel.warning:
        return Colors.orange;
      case ActivityLevel.error:
        return theme.colorScheme.error;
      case ActivityLevel.info:
        return theme.colorScheme.primary;
    }
  }
}
