import '../generated/phonebridge/v1/phonebridge.pb.dart' as pb;

/// Immutable representation of an Android notification mirrored to the desktop (DEC-028).
/// Held ephemeral in memory; zero disk persistence.
class NotificationItem {
  final String key;
  final String packageName;
  final String appName;
  final String title;
  final String text;
  final String subText;
  final DateTime postTime;
  final bool isOngoing;
  final bool isClearable;
  final String category;
  final DateTime receivedAt;

  const NotificationItem({
    required this.key,
    required this.packageName,
    required this.appName,
    required this.title,
    required this.text,
    this.subText = '',
    required this.postTime,
    this.isOngoing = false,
    this.isClearable = true,
    this.category = '',
    required this.receivedAt,
  });

  factory NotificationItem.fromProto(pb.NotificationPosted proto) {
    return NotificationItem(
      key: proto.key,
      packageName: proto.packageName,
      appName: proto.appName.isNotEmpty ? proto.appName : proto.packageName,
      title: proto.title,
      text: proto.text,
      subText: proto.subText,
      postTime: DateTime.fromMillisecondsSinceEpoch(proto.postTimeMs.toInt()),
      isOngoing: proto.isOngoing,
      isClearable: proto.isClearable,
      category: proto.category,
      receivedAt: DateTime.now(),
    );
  }

  NotificationItem copyWith({
    String? key,
    String? packageName,
    String? appName,
    String? title,
    String? text,
    String? subText,
    DateTime? postTime,
    bool? isOngoing,
    bool? isClearable,
    String? category,
    DateTime? receivedAt,
  }) {
    return NotificationItem(
      key: key ?? this.key,
      packageName: packageName ?? this.packageName,
      appName: appName ?? this.appName,
      title: title ?? this.title,
      text: text ?? this.text,
      subText: subText ?? this.subText,
      postTime: postTime ?? this.postTime,
      isOngoing: isOngoing ?? this.isOngoing,
      isClearable: isClearable ?? this.isClearable,
      category: category ?? this.category,
      receivedAt: receivedAt ?? this.receivedAt,
    );
  }

  String get timeAgo {
    final diff = DateTime.now().difference(postTime);
    if (diff.inSeconds < 45) return 'Just now';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is NotificationItem &&
          runtimeType == other.runtimeType &&
          key == other.key &&
          title == other.title &&
          text == other.text &&
          subText == other.subText &&
          isOngoing == other.isOngoing &&
          isClearable == other.isClearable;

  @override
  int get hashCode =>
      key.hashCode ^
      title.hashCode ^
      text.hashCode ^
      subText.hashCode ^
      isOngoing.hashCode ^
      isClearable.hashCode;
}
