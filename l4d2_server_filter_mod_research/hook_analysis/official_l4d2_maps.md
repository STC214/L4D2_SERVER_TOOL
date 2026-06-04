# Official L4D2 Map Codes

用于识别“默认服务器名 + 官方地图 + 超过 8 人上限”的伪装入口服启发式规则。

主要来源：

- Valve Developer Community: Left 4 Dead 2 official campaigns/maps
- Commands.gg L4D2 map list
- The Last Stand public map-code references

## 判定规则

当前 GUI 工具会自动过滤同时满足以下条件的服务器详情响应：

- `Name == "Left 4 Dead 2"`
- `Map` 是下表中的官方地图代码
- `MaxPlayers > 8`

玩家数本身不作为必要条件，因为伪装入口服也可能显示类似 `3/12`、`15/27` 的人数。

## 地图代码表

| Campaign | Code | Chapter |
|---|---|---|
| Dead Center | `c1m1_hotel` | The Hotel |
| Dead Center | `c1m2_streets` | The Streets |
| Dead Center | `c1m3_mall` | The Mall |
| Dead Center | `c1m4_atrium` | Atrium |
| Dark Carnival | `c2m1_highway` | Highway |
| Dark Carnival | `c2m2_fairgrounds` | Fairgrounds |
| Dark Carnival | `c2m3_coaster` | Coaster |
| Dark Carnival | `c2m4_barns` | Barns |
| Dark Carnival | `c2m5_concert` | Concert |
| Swamp Fever | `c3m1_plankcountry` | Plank Country |
| Swamp Fever | `c3m2_swamp` | Swamp |
| Swamp Fever | `c3m3_shantytown` | Shantytown |
| Swamp Fever | `c3m4_plantation` | Plantation |
| Hard Rain | `c4m1_milltown_a` | Milltown |
| Hard Rain | `c4m2_sugarmill_a` | Sugar Mill |
| Hard Rain | `c4m3_sugarmill_b` | Mill Escape |
| Hard Rain | `c4m4_milltown_b` | Return to Town |
| Hard Rain | `c4m5_milltown_escape` | Town Escape |
| The Parish | `c5m1_waterfront_sndscape` | Waterfront Soundscape Variant |
| The Parish | `c5m1_waterfront` | Waterfront |
| The Parish | `c5m2_park` | Park |
| The Parish | `c5m3_cemetery` | Cemetery |
| The Parish | `c5m4_quarter` | Quarter |
| The Parish | `c5m5_bridge` | Bridge |
| The Passing | `c6m1_riverbank` | Riverbank |
| The Passing | `c6m2_bedlam` | Underground |
| The Passing | `c6m3_port` | Port |
| The Sacrifice | `c7m1_docks` | Docks |
| The Sacrifice | `c7m2_barge` | Barge |
| The Sacrifice | `c7m3_port` | Port |
| No Mercy | `c8m1_apartment` | Apartments |
| No Mercy | `c8m2_subway` | Subway |
| No Mercy | `c8m3_sewers` | Sewers |
| No Mercy | `c8m4_interior` | Hospital |
| No Mercy | `c8m5_rooftop` | Rooftop |
| Crash Course | `c9m1_alleys` | Alleys |
| Crash Course | `c9m2_lots` | Truck Depot |
| Death Toll | `c10m1_caves` | Turnpike |
| Death Toll | `c10m2_drainage` | Drains |
| Death Toll | `c10m3_ranchhouse` | Church |
| Death Toll | `c10m4_mainstreet` | Town |
| Death Toll | `c10m5_houseboat` | Boathouse Finale |
| Dead Air | `c11m1_greenhouse` | Greenhouse |
| Dead Air | `c11m2_offices` | Crane |
| Dead Air | `c11m3_garage` | Construction Site |
| Dead Air | `c11m4_terminal` | Terminal |
| Dead Air | `c11m5_runway` | Runway Finale |
| Blood Harvest | `c12m1_hilltop` | Woods |
| Blood Harvest | `c12m2_traintunnel` | Tunnel |
| Blood Harvest | `c12m3_bridge` | Bridge |
| Blood Harvest | `c12m4_barn` | Train Station |
| Blood Harvest | `c12m5_cornfield` | Farmhouse Finale |
| Cold Stream | `c13m1_alpinecreek` | Alpine Creek |
| Cold Stream | `c13m2_southpinestream` | South Pine Stream |
| Cold Stream | `c13m3_memorialbridge` | Memorial Bridge |
| Cold Stream | `c13m4_cutthroatcreek` | Cut-throat Creek |
| The Last Stand | `c14m1_junkyard` | The Junkyard |
| The Last Stand | `c14m2_lighthouse` | Lighthouse Finale |

