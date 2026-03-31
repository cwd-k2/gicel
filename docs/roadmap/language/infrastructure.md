# Infrastructure — Open Questions

GIMonad と Grade 周辺の未確定設計。

## Grade の将来

| 問題                     | 現状                                 | トリガー                                 |
| ------------------------ | ------------------------------------ | ---------------------------------------- |
| `@` の型演算子降格       | `@grade` は構文糖衣                  | `□ g A` modality 導入時                  |
| Grade polymorphism       | `\(π: Mult). A @π -> B` 形式は未対応 | QTT 的 usage counting の需要             |
| Semiring law enforcement | 文書化のみ                           | grade 変数を含む多相推論が必要になった時 |

### Semiring law enforcement について

**不採用理由**: 依存型/リファイン型を導入しない設計制約に抵触。

**実害が出る条件**: checker が stuck な grade 式（meta を含む `GradeCompose ?a ?b`）に対して結合律等の法則ベースの書き換えを必要とする場合。現状は全て具体値に reduce して解決しており、grade 変数の多相推論は行っていないため実害なし。

**将来の選択肢**: grade 変数多相推論が必要になった場合、(1) rewrite rules で限定的に法則を導入、(2) 具体値 reduce のみを維持し多相推論を annotation で回避、のいずれかを選択。

## supermonad との関係

Supermonad (Bracker-Nilsson) はモナド則下で単一の indexed type に対し GIMonad に退化する。GIMonad は supermonad の正規形であり、不要な自由度 (heterogeneous bind) を持たない分、推論が決定的。
